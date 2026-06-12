// Package reflect 提供框架内部使用的反射工具函数。
package reflect

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

// GetTypeName 获取类型的名称。
func GetTypeName(v interface{}) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return "nil"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// GetFuncName 获取函数名称。
func GetFuncName(f interface{}) string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	// 移除包路径
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

// IsNil 检查值是否为 nil（支持 interface 和 pointer）。
func IsNil(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// ValidateStruct 验证结构体字段。
// 返回字段名到错误信息的映射。
func ValidateStruct(v interface{}) map[string]string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	rt := rv.Type()
	errors := make(map[string]string)

	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		value := rv.Field(i)

		// 读取 validate tag
		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			parts := strings.SplitN(rule, "=", 2)
			ruleName := parts[0]

			switch ruleName {
			case "required":
				if isZeroValue(value) {
					errors[field.Name] = fmt.Sprintf("%s is required", field.Name)
				}
			case "min":
				// 简化实现
			case "max":
				// 简化实现
			case "email":
				// 简化实现
			}
		}
	}

	return errors
}

// isZeroValue 检查值是否为零值。
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	}
	return false
}