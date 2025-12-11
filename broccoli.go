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

// command represents the internal structure for a CLI command.
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
	Env      *string      `json:"env,omitempty"`
	Alias    *string      `json:"alias,omitempty"`
	Required bool         `json:"required"`
}

// ErrTypeNotSupported is returned when a field type is not supported.
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
			if v, ok := st.Lookup("env"); ok {
				fm.Env = &v
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
var ErrHelp = errors.New("broccoli: help requested")

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
	// WrittenFields tracks which flags were explicitly set by arguments
	var WrittenFields []string = wfb[:0]
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
			rawName := name
			name = strings.TrimPrefix(name, "!")

			var Found bool = false
			for j := range cmd.Flags {
				if (hasLongPrefix && cmd.Flags[j].Name == name) ||
					(hasShortPrefix && cmd.Flags[j].Alias != nil && *cmd.Flags[j].Alias == name) {
					Found = true

					DstField := dst.Field(cmd.Flags[j].Index)
					if cmd.Flags[j].Kind == "bool" {
						var val bool
						if strings.HasPrefix(rawName, "!") {
							val = false
						} else {
							val = true
						}

						if DstField.CanSet() {
							DstField.SetBool(val)
						}
						if DstField.CanSet() {
							DstField.SetBool(val)
						}
						WrittenFields = append(WrittenFields, "--"+cmd.Flags[j].Name)

						goto skip
					}

					if i+1 >= len(args) {
						return nil, cmd, fmt.Errorf("%s requires %s", name, cmd.Flags[j].Kind)
					}
					value := args[i+1]

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
					WrittenFields = append(WrittenFields, args[i])
					i++
					break
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

	// Check Fields and Apply Defaults/Env
	for i := range cmd.Flags {
		var Found bool = false
		for j := range WrittenFields {
			if strings.HasPrefix(WrittenFields[j], "--") {
				if WrittenFields[j][2:] == cmd.Flags[i].Name {
					Found = true
					break
				}
			} else if strings.HasPrefix(WrittenFields[j], "-") {
				if cmd.Flags[i].Alias != nil && WrittenFields[j][1:] == *cmd.Flags[i].Alias {
					Found = true
					break
				}
			} else {
				// Unreachable
				panic("unreachable")
			}
		}

		// If the flag was NOT provided in arguments
		if !Found {
			// 1. Try Environment Variable
			if cmd.Flags[i].Env != nil {
				if val, ok := os.LookupEnv(*cmd.Flags[i].Env); ok {
					DstField := dst.Field(cmd.Flags[i].Index)
					err = setValue(DstField, val)
					switch err {
					case errCanNotParse:
						return nil, cmd, fmt.Errorf("can not parse (env %s) %s as %s", *cmd.Flags[i].Env, strconv.Quote(val), cmd.Flags[i].Kind)
					case errCanNotSet:
						// Ignore Error
					case nil:
						// No Error
					default:
						return nil, cmd, err
					}
					continue
				}
			}

			// 2. Try Default Value
			if cmd.Flags[i].Default != nil {
				DstField := dst.Field(cmd.Flags[i].Index)
				err = setValue(DstField, *cmd.Flags[i].Default)
				switch err {
				case errCanNotParse:
					return nil, cmd, fmt.Errorf("can not parse (default value) %s as %s", strconv.Quote(*cmd.Flags[i].Default), cmd.Flags[i].Kind)
				case errCanNotSet:
					// Ignore Error
				case nil:
					// No Error
				default:
					return nil, cmd, err
				}
				continue
			}

			// 3. Check Required
			if cmd.Flags[i].Required {
				return nil, cmd, fmt.Errorf("required parameter %s is missing", cmd.Flags[i].Name)
			}
		}
	}

	if len(args) <= 0 {
		return args[0:], cmd, nil
	}
	return args[MaxIndex+1:], cmd, nil
}

var errCanNotParse = errors.New("cannot parse value")
var errCanNotSet = errors.New("cannot set value")

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

// App represents the main application structure for the CLI.
// It holds the command configuration and provides methods to bind arguments and generate help/schema.
type App struct {
	c *command
}

// Help returns the generated help message string for the application.
// It initializes the command structure if it hasn't been initialized yet.
func (a *App) Help() string {
	a.c.init()
	return a.c.Help
}

// Schema returns the JSON representation of the command structure.
// This matches the internal structure used to define commands and flags.
func (a *App) Schema() string {
	a.c.init()
	data, err := json.Marshal(a.c)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// NewApp creates a new App instance from a struct configuration.
// v must be a pointer to a struct that defines the CLI commands and flags using tags.
// It automatically detects the executable name from the OS arguments or the executable path.
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

// Bind parses the provided arguments and sets the values in the destination struct dst.
// It returns the remaining arguments that were not parsed as flags, the App instance, and any error encountered.
func (a *App) Bind(dst interface{}, args []string) ([]string, App, error) {
	ra, cmd, err := bindCommand(a.c, args, reflect.ValueOf(dst))
	if err != nil {
		return args, App{c: cmd}, err
	}
	return ra, App{c: cmd}, nil
}

// Bind creates a new App and binds the provided arguments to the destination struct dst.
// This is a shorthand for NewApp(dst) followed by a.Bind(dst, args).
func Bind(dst interface{}, args []string) ([]string, App, error) {
	a, err := NewApp(dst)
	if err != nil {
		return args, App{}, err
	}
	return a.Bind(dst, args)
}

// BindOSArgs binds the command-line arguments (os.Args) to the destination struct dst.
// It automatically handles "--help" and version printing, exiting the program if necessary.
// If an error occurs during binding (e.g., missing required flags), it prints the error and help message to stderr and exits with status 1.
// It returns the remaining non-flag arguments.
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
			if app.c.Version != nil {
				sb.WriteRune(' ')
				sb.WriteString(*app.c.Version)
			}
			sb.WriteRune('\n')

			// Write Author
			if app.c.Author != nil {
				sb.WriteString(*app.c.Author)
				sb.WriteRune('\n')
			}

			// Write LongAbout
			if app.c.LongAbout != nil {
				sb.WriteString(*app.c.LongAbout)
				sb.WriteRune('\n')
			} else if app.c.About != nil {
				sb.WriteString(*app.c.About)
				sb.WriteRune('\n')
			}

			// Write Usage
			sb.WriteRune('\n')
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
			const helpOption = "\t-h, --help "
			var MaxLength int = len(helpOption)
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
				if a.Flags[i].Env != nil {
					sb.WriteRune(' ')
					sb.WriteString("[env: ")
					sb.WriteString(*a.Flags[i].Env)
					sb.WriteRune(']')
				}
				if a.Flags[i].Required {
					sb.WriteRune(' ')
					sb.WriteString("(required)")
				}
				sb.WriteRune('\n')
			}

			sb.WriteString(helpOption)
			for j := 0; j < MaxLength-len(helpOption); j++ {
				sb.WriteRune(' ')
			}
			sb.WriteString("Print this help message and exit")
			sb.WriteRune('\n')
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
