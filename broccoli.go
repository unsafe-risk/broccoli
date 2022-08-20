package broccoli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type command struct {
	initOnce *sync.Once `json:"-"`
	Parent   *command   `json:"-"`

	Type        reflect.Type `json:"-"`
	Command     string       `json:"command"`
	Index       int          `json:"index"`
	Author      *string      `json:"author,omitempty"`
	About       *string      `json:"about,omitempty"`
	LongAbout   *string      `json:"long_about,omitempty"`
	Version     *string      `json:"version,omitempty"`
	Flags       []fieldMeta  `json:"flags"`
	SubCommands []command    `json:"subcommands"`
	Help        string       `json:"help"`
}

type fieldMeta struct {
	Type     reflect.Type `json:"-"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	About    string       `json:"about"`
	Index    int          `json:"index"`
	Default  *string      `json:"default,omitempty"`
	Alias    *string      `json:"alias,omitempty"`
	Required bool         `json:"required"`
}

var ErrTypeNotSupported = errors.New("broccoli: type not supported")

func buildCommand(rt reflect.Type, parent *command, commandName string) (*command, error) {
	var err error

	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return nil, ErrTypeNotSupported
	}

	cmd := &command{
		initOnce: &sync.Once{},
		Parent:   parent,
		Command:  commandName,
		Type:     rt,
	}

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		st := f.Tag

		if f.Type.Kind() == reflect.Struct && f.Type.NumField() == 0 {
			if v, ok := st.Lookup("command"); ok {
				cmd.Command = v
			}
			if v, ok := st.Lookup("author"); ok {
				cmd.Author = &v
			}
			if v, ok := st.Lookup("about"); ok {
				cmd.About = &v
			}
			if v, ok := st.Lookup("long_about"); ok {
				cmd.LongAbout = &v
			}
			if v, ok := st.Lookup("version"); ok {
				cmd.Version = &v
			}
			continue
		}

		// skip unexported fields
		if !f.IsExported() {
			continue
		}

		if v, ok := st.Lookup("subcommand"); ok {
			subcmd, err := buildCommand(f.Type, cmd, v)
			if err != nil {
				return nil, err
			}
			subcmd.Index = i
			cmd.SubCommands = append(cmd.SubCommands, *subcmd)
			continue
		}

		if v, ok := st.Lookup("flag"); ok {
			var t reflect.Type = f.Type
			for t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			fm := fieldMeta{
				Name:  v,
				Kind:  t.Kind().String(),
				Index: i,
			}
			if v, ok := st.Lookup("default"); ok {
				fm.Default = &v
			}
			if v, ok := st.Lookup("alias"); ok {
				fm.Alias = &v
			}
			if v, ok := st.Lookup("required"); ok {
				fm.Required, err = strconv.ParseBool(v)
				if err != nil {
					return nil, err
				}
			}
			if v, ok := st.Lookup("about"); ok {
				fm.About = v
			}
			cmd.Flags = append(cmd.Flags, fm)
			continue
		}
	}

	return cmd, nil
}

var ErrTypeMismatch = errors.New("broccoli: type mismatch")
var ErrMissingRequiredField = errors.New("broccoli: missing required field")
var ErrHelp = errors.New("broccoli: help")

func bindCommand(cmd *command, args []string, dst reflect.Value) ([]string, *command, error) {
	cmd.init()
	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	if dst.Kind() != reflect.Struct {
		return nil, cmd, ErrTypeMismatch
	}

	if dst.Type() != cmd.Type {
		return nil, cmd, ErrTypeMismatch
	}

	if len(args) > 0 {
		// Check SubCommands
		for i := range cmd.SubCommands {
			if cmd.SubCommands[i].Command == args[0] {
				return bindCommand(&cmd.SubCommands[i], args[1:], dst.Field(cmd.SubCommands[i].Index))
			}
		}
	}

	var err error
	var wfb [32]string
	var WritedFields []string = wfb[:0]
	var MaxIndex int = 0

	for i := 0; i < len(args); i++ {
		hasLongPrefix := strings.HasPrefix(args[i], "--")
		hasShortPrefix := strings.HasPrefix(args[i], "-")

		if hasLongPrefix || hasShortPrefix {
			var name string
			if hasLongPrefix {
				name = args[i][2:]
			} else if hasShortPrefix {
				name = args[i][1:]
			} else {
				// Unreachable
				panic("unreachable")
			}
			var Found bool = false
			for j := range cmd.Flags {
				if (hasLongPrefix && cmd.Flags[j].Name == name) ||
					(hasShortPrefix && cmd.Flags[j].Alias != nil && *cmd.Flags[j].Alias == name) {
					Found = true
					if i+1 >= len(args) {
						if cmd.Flags[j].Kind == "bool" {
							Dest := dst.Field(cmd.Flags[j].Index)
							for Dest.Kind() == reflect.Pointer {
								if Dest.IsNil() {
									Dest.Set(reflect.New(Dest.Type().Elem()))
								}
								Dest = Dest.Elem()
							}
							if Dest.CanSet() {
								Dest.SetBool(true)
							}
							WritedFields = append(WritedFields, args[i])
							break
						} else {
							return nil, cmd, fmt.Errorf("%s requires %s", name, cmd.Flags[j].Kind)
						}
					}

					value := args[i+1]
					DstField := dst.Field(cmd.Flags[j].Index)
					if cmd.Flags[j].Kind == "bool" && strings.HasPrefix(value, "-") {
						if DstField.CanSet() {
							DstField.SetBool(true)
						}
						WritedFields = append(WritedFields, args[i])
						goto skip
					}
					err = setValue(DstField, value)

					switch err {
					case errCanNotParse:
						// Parse Error
						return nil, cmd, fmt.Errorf("can not parse %s as %s", strconv.Quote(value), cmd.Flags[j].Kind)
					case errCanNotSet:
						// Ignore Error
					case nil:
						// No Error
					default:
						// Unknown Error
						return nil, cmd, err
					}
					WritedFields = append(WritedFields, args[i])
					i++
				}
			}
			if !Found {
				// Handle Help
				if args[i] == "--help" || args[i] == "-h" {
					return nil, cmd, ErrHelp
				}
			}
		} else {
			break
		}

	skip:
		MaxIndex = i
	}

	// Check Required Fields
	for i := range cmd.Flags {
		if cmd.Flags[i].Required {
			var Found bool = false
			for j := range WritedFields {
				if strings.HasPrefix(WritedFields[j], "--") {
					if WritedFields[j][2:] == cmd.Flags[i].Name {
						Found = true
						break
					}
				} else if strings.HasPrefix(WritedFields[j], "-") {
					if WritedFields[j][1:] == cmd.Flags[i].Name {
						Found = true
						break
					}
				} else {
					// Unreachable
					panic("unreachable")
				}
			}
			if !Found {
				if cmd.Flags[i].Default != nil {
					DstField := dst.Field(cmd.Flags[i].Index)
					err = setValue(DstField, *cmd.Flags[i].Default)
					switch err {
					case errCanNotParse:
						// Parse Error
						return nil, cmd, fmt.Errorf("can not parse (default value) %s as %s", strconv.Quote(*cmd.Flags[i].Default), cmd.Flags[i].Kind)
					case errCanNotSet:
						// Ignore Error
					case nil:
						// No Error
					default:
						// Unknown Error
						return nil, cmd, err
					}
					continue
				}

				return nil, cmd, fmt.Errorf("required parameter %s is missing", cmd.Flags[i].Name)
			}
		}
	}

	if len(args) <= 0 {
		return args[0:], cmd, nil
	}
	return args[MaxIndex+1:], cmd, nil
}

var errCanNotParse = errors.New("can not parse")
var errCanNotSet = errors.New("can not set")

func setValue(dst reflect.Value, value string) error {
	var err error

	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	if !dst.CanSet() {
		return errCanNotSet
	}

	switch dst.Kind() {
	case reflect.String:
		dst.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var val int64
		if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") ||
			strings.HasPrefix(value, "-0x") || strings.HasPrefix(value, "-0X") ||
			strings.HasPrefix(value, "+0x") || strings.HasPrefix(value, "+0X") {
			val, err = strconv.ParseInt(value[2:], 16, 64)
		} else if strings.HasPrefix(value, "0b") || strings.HasPrefix(value, "0B") ||
			strings.HasPrefix(value, "-0b") || strings.HasPrefix(value, "-0B") ||
			strings.HasPrefix(value, "+0b") || strings.HasPrefix(value, "+0B") {
			val, err = strconv.ParseInt(value[2:], 2, 64)
		} else if strings.HasPrefix(value, "0o") || strings.HasPrefix(value, "0O") ||
			strings.HasPrefix(value, "-0o") || strings.HasPrefix(value, "-0O") ||
			strings.HasPrefix(value, "+0o") || strings.HasPrefix(value, "+0O") {
			val, err = strconv.ParseInt(value[2:], 8, 64)
		} else {
			val, err = strconv.ParseInt(value, 10, 64)
		}
		if err != nil {
			return errCanNotParse
		}
		dst.SetInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var val uint64
		if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") ||
			strings.HasPrefix(value, "-0x") || strings.HasPrefix(value, "-0X") ||
			strings.HasPrefix(value, "+0x") || strings.HasPrefix(value, "+0X") {
			val, err = strconv.ParseUint(value[2:], 16, 64)
		} else if strings.HasPrefix(value, "0b") || strings.HasPrefix(value, "0B") ||
			strings.HasPrefix(value, "-0b") || strings.HasPrefix(value, "-0B") ||
			strings.HasPrefix(value, "+0b") || strings.HasPrefix(value, "+0B") {
			val, err = strconv.ParseUint(value[2:], 2, 64)
		} else if strings.HasPrefix(value, "0o") || strings.HasPrefix(value, "0O") ||
			strings.HasPrefix(value, "-0o") || strings.HasPrefix(value, "-0O") ||
			strings.HasPrefix(value, "+0o") || strings.HasPrefix(value, "+0O") {
			val, err = strconv.ParseUint(value[2:], 8, 64)
		} else {
			val, err = strconv.ParseUint(value, 10, 64)
		}
		if err != nil {
			return errCanNotParse
		}
		dst.SetUint(val)
	case reflect.Float32, reflect.Float64:
		var val float64
		val, err = strconv.ParseFloat(value, 64)
		if err != nil {
			return errCanNotParse
		}
		dst.SetFloat(val)
	case reflect.Bool:
		var val bool
		val, err = strconv.ParseBool(value)
		if err != nil {
			return errCanNotParse
		}
		dst.SetBool(val)
	case reflect.Slice:
		val := strings.Split(value, ",")
		if dst.Cap() < len(val) {
			dst.Set(reflect.MakeSlice(dst.Type(), len(val), len(val)))
		} else {
			dst.SetLen(len(val))
		}
		for i := 0; i < len(val); i++ {
			err = setValue(dst.Index(i), val[i])
			if err != nil {
				return err
			}
		}
	}
	return err
}

type App struct {
	c *command
}

func (a *App) Help() string {
	a.c.init()
	return a.c.Help
}

func (a *App) Schema() string {
	a.c.init()
	data, err := json.Marshal(a.c)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func NewApp(v interface{}) (*App, error) {
	rv := reflect.ValueOf(v)
	exe, err := os.Executable()
	if err != nil {
		if len(os.Args) > 0 {
			exe = os.Args[0]
		} else {
			exe = "unknown"
		}
	}
	exe = strings.TrimSuffix(exe, ".exe")
	exe = filepath.Base(exe)
	cmd, err := buildCommand(rv.Type(), nil, exe)
	if err != nil {
		return nil, err
	}
	cmd.init()
	return &App{c: cmd}, nil
}

func (a *App) Bind(dst interface{}, args []string) ([]string, App, error) {
	ra, cmd, err := bindCommand(a.c, args, reflect.ValueOf(dst))
	if err != nil {
		return args, App{c: cmd}, err
	}
	return ra, App{c: cmd}, nil
}

func Bind(dst interface{}, args []string) ([]string, App, error) {
	a, err := NewApp(dst)
	if err != nil {
		return args, App{}, err
	}
	return a.Bind(dst, args)
}

// BindOSArgs bind os args to struct
func BindOSArgs(dst interface{}) []string {
	a, err := NewApp(dst)
	if err != nil {
		panic(err)
	}
	a.c.init()
	ra, app, err := a.Bind(dst, os.Args[1:])
	if err != nil {
		if err == ErrHelp {
			var sb strings.Builder

			// Write Command and Version
			sb.WriteString(app.c.Command)
			if a.c.Version != nil {
				sb.WriteRune(' ')
				sb.WriteString(*app.c.Version)
			}
			sb.WriteRune('\n')

			// Write Author
			if a.c.Author != nil {
				sb.WriteString(*app.c.Author)
				sb.WriteRune('\n')
			}

			// Write LongAbout
			if a.c.LongAbout != nil {
				sb.WriteString(*app.c.LongAbout)
				sb.WriteRune('\n')
			}

			// Write Usage
			sb.WriteString(app.Help())

			fmt.Print(sb.String())
			os.Exit(0)
		}

		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, app.Help())
		os.Exit(1)
	}
	return ra
}

func (a *command) init() {
	a.initOnce.Do(func() {
		var sb strings.Builder

		// Write Usage
		sb.WriteString("Usage: \n\t")
		var parent *command
		var subcommandPath []string
		if a.Parent != nil {
			for parent = a; parent != nil; parent = parent.Parent {
				subcommandPath = append(subcommandPath, parent.Command)
			}
			for i := len(subcommandPath) - 1; i >= 0; i-- {
				sb.WriteString(subcommandPath[i])
				if i > 0 {
					sb.WriteRune(' ')
				}
			}
		} else {
			sb.WriteString(a.Command)
		}

		if len(a.SubCommands) > 0 {
			sb.WriteString(" <COMMAND>")
		}

		if len(a.Flags) > 0 {
			sb.WriteString(" [OPTIONS]")
			for i := range a.Flags {
				if a.Flags[i].Required {
					sb.WriteRune(' ')
					sb.WriteRune('-')
					sb.WriteRune('-')
					sb.WriteString(a.Flags[i].Name)
					sb.WriteRune(' ')
					sb.WriteRune('<')
					sb.WriteString(strings.ToUpper(a.Flags[i].Name))
					sb.WriteRune('>')
				}
			}
		}
		sb.WriteString(" [ARGUEMENTS]\n\n")

		// Write Options
		if len(a.Flags) > 0 {
			sb.WriteString("Options:\n")
			var CommandNames []string = make([]string, len(a.Flags))
			for i := range a.Flags {
				var ssb strings.Builder

				ssb.WriteString("\t")
				if a.Flags[i].Alias != nil {
					ssb.WriteRune('-')
					ssb.WriteString(*a.Flags[i].Alias)
					ssb.WriteRune(',')
					ssb.WriteRune(' ')
				}

				ssb.WriteRune('-')
				ssb.WriteRune('-')
				ssb.WriteString(a.Flags[i].Name)
				ssb.WriteRune(' ')

				CommandNames[i] = ssb.String()
			}

			var MaxLength int = 0
			for i := range CommandNames {
				if len(CommandNames[i]) > MaxLength {
					MaxLength = len(CommandNames[i])
				}
			}
			MaxLength += 4

			for i := range a.Flags {
				sb.WriteString(CommandNames[i])
				for j := 0; j < MaxLength-len(CommandNames[i]); j++ {
					sb.WriteRune(' ')
				}
				sb.WriteString(a.Flags[i].About)
				sb.WriteRune(' ')
				if a.Flags[i].Default != nil {
					sb.WriteString("[default: ")
					sb.WriteString(*a.Flags[i].Default)
					sb.WriteRune(']')
				}
				if a.Flags[i].Required {
					sb.WriteRune(' ')
					sb.WriteString("(required)")
				}
				sb.WriteRune('\n')
			}
		}
		sb.WriteRune('\n')

		// Write SubCommands
		if len(a.SubCommands) > 0 {
			sb.WriteString("Commands:\n")
			var MaxLength int = 0
			for i := range a.SubCommands {
				if len(a.SubCommands[i].Command) > MaxLength {
					MaxLength = len(a.SubCommands[i].Command)
				}
			}
			MaxLength += 4

			for i := range a.SubCommands {
				sb.WriteString("\t")
				sb.WriteString(a.SubCommands[i].Command)

				for j := 0; j < MaxLength-len(a.SubCommands[i].Command); j++ {
					sb.WriteRune(' ')
				}

				if a.SubCommands[i].About != nil {
					sb.WriteString(*a.SubCommands[i].About)
				}
				sb.WriteRune('\n')
			}
		}

		a.Help = sb.String()
	})

	for i := range a.SubCommands {
		a.SubCommands[i].Parent = a
		a.SubCommands[i].init()
	}
}
