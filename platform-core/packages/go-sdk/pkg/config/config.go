package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Load fills a T from the process environment. The returned error names every
// variable that failed to parse or is required and unset. After parsing, Load
// calls Validate() error on T when it has one and on every group (nested or
// embedded struct field), and joins all the errors. Embedded groups that each
// define Validate do not promote it to T, so the walk is what reaches them.
//
// A string field tagged keepempty:"true" stays empty when its variable is set
// to the empty string, instead of taking its default. Unset still applies the
// default.
//
// A field with a default tag is never reported as unset: caarlos0/env applies
// the default before it checks "required".
func Load[T any]() (T, error) {
	v, err := env.ParseAsWithOptions[T](env.Options{
		DefaultValueTagName: "default",
		RequiredIfNoDef:     false,
	})
	if err != nil {
		return v, nameVariables(err, reflect.TypeFor[T]())
	}
	keepEmpty(reflect.ValueOf(&v).Elem())
	if err := validateAll(reflect.ValueOf(v)); err != nil {
		return v, err
	}
	return v, nil
}

type validator interface{ Validate() error }

// validateAll runs Validate on v and on every struct field below it, joining
// the errors. A promoted Validate and the group it comes from report the same
// message, so identical messages are kept once.
func validateAll(v reflect.Value) error {
	var errs []error
	seen := map[string]struct{}{}
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if val, ok := v.Interface().(validator); ok {
			if err := val.Validate(); err != nil {
				if _, dup := seen[err.Error()]; !dup {
					seen[err.Error()] = struct{}{}
					errs = append(errs, err)
				}
			}
		}
		if v.Kind() != reflect.Struct {
			return
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() && v.Field(i).Kind() == reflect.Struct {
				walk(v.Field(i))
			}
		}
	}
	walk(v)
	return errors.Join(errs...)
}

// nameVariables rewrites parse errors, which env reports by Go field name, to
// name the environment variable instead. Several nested fields can share a Go
// name (Enabled), so each candidate is re-parsed alone to find the culprit.
func nameVariables(err error, t reflect.Type) error {
	var agg env.AggregateError
	if !errors.As(err, &agg) {
		return err
	}
	fields := envFields(t)
	out := make([]error, 0, len(agg.Errors))
	for _, e := range agg.Errors {
		var pe env.ParseError
		if !errors.As(e, &pe) {
			out = append(out, e)
			continue
		}
		named := false
		for _, f := range fields {
			if f.name != pe.Name || f.typ != pe.Type {
				continue
			}
			if probe := probeField(f); probe != nil {
				out = append(out, fmt.Errorf("%s: %w", f.key, pe.Err))
				named = true
			}
		}
		if !named {
			out = append(out, e)
		}
	}
	return errors.Join(out...)
}

// probeField parses a single field on its own and returns its error.
func probeField(f envField) error {
	st := reflect.StructOf([]reflect.StructField{{Name: "F", Type: f.typ, Tag: f.tag}})
	ptr := reflect.New(st)
	return env.ParseWithOptions(ptr.Interface(), env.Options{DefaultValueTagName: "default"})
}

// keepEmpty resets every string field tagged keepempty:"true" to "" when its
// variable is set to the empty string, recursing into nested structs. The env
// binder treats an empty value like an unset one and applies the default.
func keepEmpty(v reflect.Value) {
	t := v.Type()
	for i := range t.NumField() {
		f, fv := t.Field(i), v.Field(i)
		if !f.IsExported() {
			continue
		}
		if tag, ok := f.Tag.Lookup("env"); ok {
			key, _, _ := strings.Cut(tag, ",")
			if f.Tag.Get("keepempty") != "true" || fv.Kind() != reflect.String {
				continue
			}
			if val, set := os.LookupEnv(key); set && val == "" {
				fv.SetString("")
			}
			continue
		}
		if fv.Kind() == reflect.Struct {
			keepEmpty(fv)
		}
	}
}
