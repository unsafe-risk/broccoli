package broccoli

import (
	"math"
	"reflect"
	"testing"
)

func TestBindArgs(t *testing.T) {
	t.Run("test-no-args", func(t *testing.T) {
		type NoArgsApp struct {
			_ struct{} `version:"1.0.0" command:"NoArgsApp" about:"This is a test app"`
		}
		var app NoArgsApp
		args, _, err := Bind(&app, []string{})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}
	})

	t.Run("test-flags", func(t *testing.T) {
		type LongFlagApp struct {
			_         struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			FirstName string   `flag:"name" about:"Your first name"`
			LastName  string   `flag:"last" about:"Your last name"`

			Age int `flag:"age" about:"Your age"`
		}
		var app LongFlagApp
		args, _, err := Bind(&app, []string{"--name", "John", "--last", "Doe", "--age", "42"})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d\n%v", len(args), args)
		}

		if app.FirstName != "John" {
			t.Errorf("expected first name to be 'John', got '%s'", app.FirstName)
		}

		if app.LastName != "Doe" {
			t.Errorf("expected last name to be 'Doe', got '%s'", app.LastName)
		}

		if app.Age != 42 {
			t.Errorf("expected age to be 42, got %d", app.Age)
		}
	})

	t.Run("test-required-flags", func(t *testing.T) {
		type RequiredFlagsApp struct {
			_    struct{} `version:"1.0.0" command:"RequiredFlagsApp" about:"This is a test app"`
			Name string   `flag:"name" alias:"n" required:"true" about:"Your first name"`
			Age  int      `flag:"age" alias:"a" required:"true" about:"Your age"`
		}
		var app RequiredFlagsApp
		_, _, err := Bind(&app, []string{})
		if err == nil {
			t.Error("expected ErrMissingRequiredField, got nil")
		}
	})

	t.Run("test-default-flags", func(t *testing.T) {
		type DefaultFlagsApp struct {
			_    struct{} `version:"1.0.0" command:"DefaultFlagsApp" about:"This is a test app"`
			Name string   `flag:"name" alias:"n" required:"true" about:"Your first name" default:"John"`
			Age  int      `flag:"age" alias:"a" required:"true" about:"Your age" default:"42"`
		}
		var app DefaultFlagsApp
		args, _, err := Bind(&app, []string{})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}

		if app.Name != "John" {
			t.Errorf("expected name to be 'John', got '%s'", app.Name)
		}

		if app.Age != 42 {
			t.Errorf("expected age to be 42, got %d", app.Age)
		}
	})

	t.Run("test-subcommand", func(t *testing.T) {
		type AddApp struct {
			_ struct{} `version:"1.0.0" command:"add" about:"This is a test app"`
			A int      `flag:"a" alias:"a" required:"true" about:"A"`
			B int      `flag:"b" alias:"b" required:"true" about:"B"`
		}
		type SubcommandApp struct {
			_    struct{} `version:"1.0.0" command:"SubcommandApp" about:"This is a test app"`
			Name string   `flag:"name" alias:"n" required:"true" about:"Your first name" default:"John"`
			Age  int      `flag:"age" alias:"a" required:"true" about:"Your age" default:"42"`
			Add  *AddApp  `subcommand:"add" about:"Add two numbers"`
		}

		var app SubcommandApp
		args, _, err := Bind(&app, []string{"add", "--a", "1", "--b", "2"})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}

		if app.Add == nil {
			t.Error("expected Add to be non-nil")
		}

		if app.Add.A != 1 {
			t.Errorf("expected Add.A to be 1, got %d", app.Add.A)
		}

		if app.Add.B != 2 {
			t.Errorf("expected Add.B to be 2, got %d", app.Add.B)
		}
	})

	t.Run("test-args", func(t *testing.T) {
		type TestApp struct {
			_         struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			FirstName string   `flag:"name" about:"Your first name"`

			Age int `flag:"age" about:"Your age"`
		}
		var app TestApp
		args, _, err := Bind(&app, []string{"--name", "John Doe", "--age", "42", "extra", "args"})
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(args, []string{"extra", "args"}) {
			t.Errorf("expected args to be ['extra', 'args'], got %v", args)
		}
	})

	t.Run("test-alias", func(t *testing.T) {
		type TestApp struct {
			_         struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			FirstName string   `flag:"name" alias:"n" about:"Your first name"`

			Age int `flag:"age" alias:"a" about:"Your age"`
		}
		var app TestApp
		args, _, err := Bind(&app, []string{"-n", "John Doe", "--age", "42", "extra", "args"})
		if err != nil {
			t.Error(err)
		}
		expectedArgs := []string{"extra", "args"}
		if !reflect.DeepEqual(args, expectedArgs) {
			t.Errorf("expected args to be %v, got %v", expectedArgs, args)
		}

		if app.FirstName != "John Doe" {
			t.Errorf("expected first name to be 'John Doe', got '%s'", app.FirstName)
		}

		if app.Age != 42 {
			t.Errorf("expected age to be 42, got %d", app.Age)
		}
	})

	t.Run("test-boolean-flags", func(t *testing.T) {
		type TestApp struct {
			_     struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			Bool0 bool     `flag:"bool0" about:"A boolean flag"`
			Bool1 bool     `flag:"bool1" about:"A boolean flag"`
			Bool2 bool     `flag:"bool2" about:"A boolean flag"`
		}
		var app TestApp
		args, _, err := Bind(&app, []string{"--bool0", "--!bool1", "--bool2"})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}

		if !app.Bool0 {
			t.Error("expected Bool0 to be true")
		}

		if app.Bool1 {
			t.Error("expected Bool1 to be false")
		}

		if !app.Bool2 {
			t.Error("expected Bool2 to be false")
		}
	})

	t.Run("test-default-boolean-flags", func(t *testing.T) {
		type TestApp struct {
			_     struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			Bool0 bool     `flag:"bool0" about:"A boolean flag" default:"true" alias:"b0"`
			Bool1 bool     `flag:"bool1" about:"A boolean flag" default:"false" alias:"b1"`
			Bool2 bool     `flag:"bool2" about:"A boolean flag" default:"true" alias:"b2"`
		}
		var app TestApp
		args, _, err := Bind(&app, []string{"-!b0", "-b1", "-b2"})
		if err != nil {
			t.Error(err)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}

		if app.Bool0 {
			t.Error("expected Bool0 to be false")
		}
		if !app.Bool1 {
			t.Error("expected Bool1 to be true")
		}
		if !app.Bool2 {
			t.Error("expected Bool2 to be true")
		}
	})

	t.Run("test-flags", func(t *testing.T) {
		type TestApp struct {
			_    struct{} `version:"1.0.0" command:"LongFlagApp" about:"This is a test app"`
			Name string   `flag:"name" about:"Your name" required:"true"`
			Age  int      `flag:"age" about:"Your age" required:"true"`

			Addr *string `flag:"addr" about:"Your address"`
			Zip  *uint64 `flag:"zip" about:"Your zip code"`

			Height float64 `flag:"height" about:"Your height"`
			Weight float32 `flag:"weight" about:"Your weight"`

			IsDeveloper bool `flag:"dev" alias:"d" about:"Are you a developer?" default:"false"`

			Clothes []string `flag:"clothes" alias:"c" about:"Your clothes" required:"true"`
		}

		var app TestApp
		args, _, err := Bind(&app, []string{"--name", "John Doe",
			"--age", "42",
			"--addr", "123 Main St",
			"--zip", "12345",
			"--height", "1.78",
			"--weight", "80.5",
			"--dev",
			"-c", "shirt,pants,hat",
			"extra", "args"})
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(args, []string{"extra", "args"}) {
			t.Errorf("expected args to be ['extra', 'args'], got %v", args)
		}

		if app.Name != "John Doe" {
			t.Errorf("expected name to be 'John Doe', got '%s'", app.Name)
		}

		if app.Age != 42 {
			t.Errorf("expected age to be 42, got %d", app.Age)
		}

		if *app.Addr != "123 Main St" {
			t.Errorf("expected addr to be '123 Main St', got '%s'", *app.Addr)
		}

		if *app.Zip != 12345 {
			t.Errorf("expected zip to be 12345, got %d", *app.Zip)
		}

		if !floatCompare(app.Height, 1.78) {
			t.Errorf("expected height to be 1.78, got %f", app.Height)
		}

		if !floatCompare(float64(app.Weight), 80.5) {
			t.Errorf("expected weight to be 80.5, got %f", app.Weight)
		}

		if !app.IsDeveloper {
			t.Error("expected IsDeveloper to be true")
		}

		if !reflect.DeepEqual(app.Clothes, []string{"shirt", "pants", "hat"}) {
			t.Errorf("expected clothes to be ['shirt', 'pants', 'hat'], got %v", app.Clothes)
		}
	})
}

func floatCompare(a, b float64) bool {
	return math.Abs(a-b) < 0.00001
}
