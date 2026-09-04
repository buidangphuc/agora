// Package config provides reflection-based environment loading and drift checking.
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Load populates targetStruct from OS environment variables based on `env:` and `default:` tags.
func Load(targetStruct interface{}) error {
	v := reflect.ValueOf(targetStruct)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("targetStruct must be a pointer to a struct")
	}
	return populateStruct(v.Elem())
}

func populateStruct(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldVal := v.Field(i)
		fieldType := t.Field(i)

		// Recurse into nested structs
		if fieldVal.Kind() == reflect.Struct {
			if err := populateStruct(fieldVal); err != nil {
				return err
			}
			continue
		}

		envKey := fieldType.Tag.Get("env")
		defaultVal := fieldType.Tag.Get("default")

		rawVal := ""
		if envKey != "" {
			rawVal = os.Getenv(envKey)
		}
		if rawVal == "" {
			rawVal = defaultVal
		}

		if rawVal != "" {
			if err := setFieldValue(fieldVal, rawVal); err != nil {
				return fmt.Errorf("field %s (%s): %w", fieldType.Name, envKey, err)
			}
		}
	}
	return nil
}

func setFieldValue(field reflect.Value, raw string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
			return nil
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(raw, ",")
			var items []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					items = append(items, p)
				}
			}
			field.Set(reflect.ValueOf(items))
		}
	}
	return nil
}
