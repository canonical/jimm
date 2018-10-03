// Copyright 2018 Canonical Ltd.

// jimmcfg is a tool to help generate and update configuration files for
// a JIMM server.
//
// To use the tool use a command like one of the following:
// 	jimmcfg -file <config file> -defaults
//	jimmcfg -file <config file> <key>
//	jimmcfg -file <config file> <key> <value>
// The first command will create, or update, a config file at the given
// path and fill out appropriate defaults for any required fields.
//
// The second form prints the current value of the configuration key from
// the specified config file.
//
// The third form will create, or update, the specified config file
// changing the value of key to value.
package main

import (
	"encoding"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/satori/uuid"
	yaml "gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/jimm/config"
)

var (
	defaults = flag.Bool("defaults", false, "Generate default values for required keys that are empty.")
	file     = flag.String("file", "", "Name of configuration `file`.")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s -file <config file> -defaults\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "%s -file <config file> <key>\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "%s -file <config file> <key> <value>\n", os.Args[0])
		os.Exit(2)
	}
	flag.Parse()
	if *file == "" {
		flag.Usage()
	}
	var confFunc func(*config.Config) error
	var write bool
	switch {
	case *defaults:
		confFunc = generate
		write = true
	case flag.NArg() == 1:
		confFunc = get
	case flag.NArg() == 2:
		confFunc = set
		write = true
	default:
		flag.Usage()
	}

	if err := run(*file, confFunc, write); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

}

// run opens the given file parses it and runs the given config function.
// If write is specified the config is written back to the same file. The
// file will be created if it does not exist. Note if there is an error
// the file will not be closed, it is assumed in this case that the
// command will exit so this isn't a problem.
func run(file string, confFunc func(*config.Config) error, write bool) error {
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	var conf config.Config
	if err := yaml.Unmarshal(buf, &conf); err != nil {
		return fmt.Errorf("invalid configuration file %q: %s", file, err)
	}
	if err := confFunc(&conf); err != nil {
		return err
	}
	if write {
		buf, err := yaml.Marshal(&conf)
		if err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err := f.Write(buf); err != nil {
			return err
		}
		if err := f.Truncate(int64(len(buf))); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

// generate creates default configuration values for required keys that
// are currently not set and a generated default makes sense.
func generate(conf *config.Config) error {
	if conf.MongoAddr == "" {
		conf.MongoAddr = "localhost:27017"
	}
	if conf.APIAddr == "" {
		conf.APIAddr = ":8080"
	}
	if conf.IdentityLocation == "" {
		conf.IdentityLocation = "https://api.jujucharms.com/identity"
	}
	if conf.ControllerUUID == "" {
		conf.ControllerUUID = uuid.NewV4().String()
	}
	return nil
}

// get retrieves the current value of a key from conf.
func get(conf *config.Config) error {
	index := configFields[flag.Arg(0)]
	if index == nil {
		return fmt.Errorf("unknown option %q", flag.Arg(0))
	}
	v := value(reflect.ValueOf(conf), index)
	if tm, ok := v.Interface().(encoding.TextMarshaler); ok {
		buf, err := tm.MarshalText()
		if err != nil {
			return err
		}
		os.Stdout.Write(append(buf, '\n'))
		return nil
	}
	fmt.Println(v.Interface())
	return nil
}

// set updates conf to set the value of key to that specified on the command line.
func set(conf *config.Config) error {
	index := configFields[flag.Arg(0)]
	if index == nil {
		return fmt.Errorf("unknown option %q", flag.Arg(0))
	}
	v := value(reflect.ValueOf(conf), index)
	if tu, ok := v.Interface().(encoding.TextUnmarshaler); ok {
		return tu.UnmarshalText([]byte(flag.Arg(1)))
	}
	if tu, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
		return tu.UnmarshalText([]byte(flag.Arg(1)))
	}
	switch v.Kind() {
	case reflect.Bool:
		b, err := strconv.ParseBool(flag.Arg(1))
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int:
		n, err := strconv.ParseInt(flag.Arg(1), 0, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.String:
		v.SetString(flag.Arg(1))
	default:
		return fmt.Errorf("unsuported type for %s", flag.Arg(0))
	}
	return nil
}

// value gets the value at the specified index. The returned value will
// be addressable and settable.
func value(v reflect.Value, index []int) reflect.Value {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return value(v.Elem(), index)
	}
	if len(index) == 0 {
		return v
	}
	return value(v.Field(index[0]), index[1:])
}

// configFields contains the indicies of all the configurable fields.
var configFields = func() map[string][]int {
	fields := make(map[string][]int)
	structFields(fields, reflect.TypeOf(config.Config{}), "", nil)
	return fields
}()

// structFields calculates the field names and indicies for all the
// exported fields in the given type, which must reflect a struct.
func structFields(fields map[string][]int, t reflect.Type, name string, prefix []int) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		prefix := append(prefix, i)
		fname := ""
		if !f.Anonymous {
			fname = f.Name
		}
		if p := strings.Split(f.Tag.Get("yaml"), ","); p[0] != "" {
			fname = p[0]
		} else if p := strings.Split(f.Tag.Get("json"), ","); p[0] != "" {
			fname = p[0]
		}
		if name != "" {
			fname = name + "." + fname
		}
		typeFields(fields, f.Type, fname, prefix)
	}
}

var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// structFields calculates the field names and indicies for the given type.
func typeFields(fields map[string][]int, t reflect.Type, name string, prefix []int) {
	if t.Implements(textUnmarshalerType) || reflect.PtrTo(t).Implements(textUnmarshalerType) {
		// Here it is assumed that if the type implements
		// encoding.TextUnmarshaler it also implements
		// encoding.TextMarshaler this is true for all
		// types that are currently used, but it will
		// panic if that becomes untrue in the future.
		fields[name] = prefix
		return
	}
	switch t.Kind() {
	case reflect.Ptr:
		typeFields(fields, t.Elem(), name, prefix)
	case reflect.Struct:
		structFields(fields, t, name, prefix)
	case reflect.Bool, reflect.Int, reflect.String:
		fields[name] = prefix
	default:
		panic(fmt.Errorf("unsupport type %v", t))
	}
}
