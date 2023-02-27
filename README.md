[![GitHub](https://img.shields.io/github/license/unsafe-risk/broccoli?style=for-the-badge)](https://github.com/unsafe-risk/broccoli/blob/main/LICENSE)
[![GitHub Workflow Status (event)](https://img.shields.io/github/actions/workflow/status/unsafe-risk/broccoli/go-test.yml?event=push&style=for-the-badge)](https://github.com/unsafe-risk/broccoli/actions/workflows/go-test.yml)
[![Go Reference](https://img.shields.io/badge/go-reference-%23007d9c?style=for-the-badge&logo=go)](https://pkg.go.dev/gopkg.eu.org/broccoli)

# broccoli

Broccoli: [CLI](https://en.wikipedia.org/wiki/Command-line_interface) Package for Go

## Usage

```go
package main

import (
	"fmt"

	"v8.run/go/broccoli"
)

type Config struct {
	_    struct{} `version:"0.0.1" command:"hello" about:"Test App"`
	Name string   `flag:"name" alias:"n" required:"true" about:"Your name"`

	Sub *SubCommand `subcommand:"sub"`
}

type SubCommand struct {
	_    struct{} `command:"sub" longabout:"Test Sub Command"`
	Name string   `flag:"name" alias:"n" required:"true" about:"Your name"`
}

func main() {
	var cfg Config
	_ = broccoli.BindOSArgs(&cfg)

	if cfg.Sub != nil {
		fmt.Printf("Hello %s from sub command\n", cfg.Sub.Name)
		return
	}

	fmt.Printf("Hello %s from main command\n", cfg.Name)
}
```

```
$ hello --help
hello 0.0.1
Test App

Usage:
        hello <COMMAND> [OPTIONS] --name <NAME> [ARGUEMENTS]

Options:
        -n, --name     Your name  (required)
        -h, --help     Print this help message and exit

Commands:
        sub    Test Sub Command

$ hello --name World
Hello World from main command

$ hello sub --name World
Hello World from sub command
```

## Installation

```bash
go get -u gopkg.eu.org/broccoli
```
