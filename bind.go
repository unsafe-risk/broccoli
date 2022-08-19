package broccoli

import (
	"strings"
	"sync"
)

type None struct{}

type command struct {
	initOnce *sync.Once `json:"-"`
	Parent   *command   `json:"-"`

	Command     string      `json:"command"`
	Author      *string     `json:"author,omitempty"`
	About       *string     `json:"about,omitempty"`
	LongAbout   *string     `json:"long_about,omitempty"`
	Version     *string     `json:"version,omitempty"`
	Flags       []fieldMeta `json:"flag"`
	SubCommands []command   `json:"subcommands"`
	Help        string      `json:"help"`
}

type fieldMeta struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	About    string  `json:"about"`
	Index    int     `json:"index"`
	Default  *string `json:"default,omitempty"`
	Alias    *string `json:"alias"`
	Required bool    `json:"required"`
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
				sb.WriteRune('\n')
			}
		}
		sb.WriteRune('\n')

		// Write SubCommands
		if len(a.SubCommands) > 0 {
			sb.WriteString("The Commands are:\n")
			for i := range a.SubCommands {
				sb.WriteString("\t")
				sb.WriteString(a.SubCommands[i].Command)
				if a.SubCommands[i].About != nil {
					sb.WriteRune(' ')
					sb.WriteString(*a.SubCommands[i].About)
				}
				sb.WriteRune('\n')
			}
		}
	})

	for i := range a.SubCommands {
		a.SubCommands[i].Parent = a
		a.SubCommands[i].init()
	}
}
