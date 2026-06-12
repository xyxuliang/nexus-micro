// Package util 提供框架内部使用的工具函数。
package util

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// GenerateID 生成一个随机的 32 字符十六进制 ID。
// 用于 request_id 和 trace_id 的生成。
func GenerateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ToSnakeCase 将 PascalCase 或 camelCase 转换为 snake_case。
func ToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// ToCamelCase 将 snake_case 转换为 PascalCase。
func ToCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// GoTypeToProto 将 Go 类型转换为 Proto 类型。
func GoTypeToProto(goType string) string {
	// 处理切片类型
	if strings.HasPrefix(goType, "[]") {
		inner := strings.TrimPrefix(goType, "[]")
		return "repeated " + GoTypeToProto(inner)
	}

	switch goType {
	case "string":
		return "string"
	case "int", "int32":
		return "int32"
	case "int64":
		return "int64"
	case "float32":
		return "float"
	case "float64":
		return "double"
	case "bool":
		return "bool"
	case "[]byte":
		return "bytes"
	default:
		return "string"
	}
}