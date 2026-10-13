package config

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/caarlos0/env/v11"
)

// Load fills a T from the process environment. The returned error names every
// variable that failed to parse or is required and unset. When T has a
// Validate() error method, Load calls it after parsing and returns its error.
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
	if val, ok := any(v).(interface{ Validate() error }); ok {
		if err := val.Validate(); err != nil {
			return v, err
		}
	}
	return v, nil
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
