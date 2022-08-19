package broccoli

import (
	"errors"
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

	Command     string      `json:"command"`
	Author      *string     `json:"author,omitempty"`
	About       *string     `json:"about,omitempty"`
	LongAbout   *string     `json:"long_about,omitempty"`
	Version     *string     `json:"version,omitempty"`
	Flags       []fieldMeta `json:"flags"`
	SubCommands []command   `json:"subcommands"`
	Help        string      `json:"help"`
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

var ErrTypeNotSupported = errors.New("type not supported")

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
			cmd.SubCommands = append(cmd.SubCommands, *subcmd)
			continue
		}

		if v, ok := st.Lookup("flag"); ok {
			fm := fieldMeta{
				Name:  v,
				Kind:  f.Type.Kind().String(),
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

type App struct {
	c *command
}

func (a *App) Help() string {
	a.c.init()
	return a.c.Help
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

func (a *command) init() {
	a.initOnce.Do(func() {
		var sb strings.Builder

		// Write Command and Version
		sb.WriteString(a.Command)
		if a.Version != nil {
			sb.WriteRune(' ')
			sb.WriteString(*a.Version)
		}
		sb.WriteRune('\n')

		// Write Author
		if a.Author != nil {
			sb.WriteString(*a.Author)
			sb.WriteRune('\n')
		}

		// Write LongAbout
		if a.LongAbout != nil {
			sb.WriteString(*a.LongAbout)
			sb.WriteRune('\n')
		}

		sb.WriteRune('\n')

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
