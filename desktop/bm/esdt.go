package bm

import (
	"fmt"
	"net/url"
	"strings"
)

// Esdt 站点密码/参数混淆算法：逐字符码点拼接 + "^" + 各码点字符串长度表（逗号分隔）
// 与站点 JS Esdt()、App 端 BmHttpLogin.esdtRaw 逐字对应
func Esdt(code string) string {
	var c strings.Builder
	lengths := make([]string, 0, len(code))
	for _, ch := range code {
		t := int(ch)
		lengths = append(lengths, fmt.Sprintf("%d", len(fmt.Sprintf("%d", t))))
		c.WriteString(fmt.Sprintf("%d", t))
	}
	return c.String() + "^" + strings.Join(lengths, ",")
}

// JSEscape JS escape() 等价实现：字母数字与 @*_+-./ 不编码，ASCII 用 %XX，非 ASCII 用 %uXXXX
func JSEscape(s string) string {
	var sb strings.Builder
	for _, ch := range s {
		c := int(ch)
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			sb.WriteRune(ch)
		case strings.ContainsRune("@*_+-./", ch):
			sb.WriteRune(ch)
		case c < 256:
			sb.WriteString(fmt.Sprintf("%%%02X", c))
		default:
			sb.WriteString(fmt.Sprintf("%%u%04X", c))
		}
	}
	return sb.String()
}

// FormEscape 对表单值先 Esdt 再 JSEscape（站点提交 Esdt 值时的完整编码链）
func FormEscape(raw string) string {
	return JSEscape(Esdt(raw))
}

// QueryEscapePid 积分接口的 pid 参数（Esdt 编码后需再按 URL 规则转义）
func QueryEscapePid(pid string) string {
	return url.QueryEscape(Esdt(pid))
}
