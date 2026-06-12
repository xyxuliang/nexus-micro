// Package lexer 提供 .api DSL 的词法分析器。
// 将输入的源码文本转换为 token 流，供 Parser 进行语法分析。
// DSL 语法借鉴 go-zero 的 .api 语法，但增加了 @grpc 注解支持。
package lexer

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// TokenType 表示 token 的类型。
type TokenType int

const (
	// TokenEOF 表示输入结束。
	TokenEOF TokenType = iota
	// TokenError 表示词法错误。
	TokenError
	// TokenComment 表示注释。
	TokenComment
	// TokenIdent 表示标识符（类型名、服务名、方法名）。
	TokenIdent
	// TokenString 表示字符串字面量。
	TokenString
	// TokenNumber 表示数字字面量。
	TokenNumber
	// TokenLParen 表示左括号 '('。
	TokenLParen
	// TokenRParen 表示右括号 ')'。
	TokenRParen
	// TokenLBrace 表示左大括号 '{'。
	TokenLBrace
	// TokenRBrace 表示右大括号 '}'。
	TokenRBrace
	// TokenLBracket 表示左方括号 '['。
	TokenLBracket
	// TokenRBracket 表示右方括号 ']'。
	TokenRBracket
	// TokenAssign 表示赋值 '='。
	TokenAssign
	// TokenArrow 表示箭头 '->'。
	TokenArrow
	// TokenComma 表示逗号 ','。
	TokenComma
	// TokenColon 表示冒号 ':'。
	TokenColon
	// TokenSlash 表示斜杠 '/'。
	TokenSlash
	// TokenDot 表示点 '.'。
	TokenDot
	// TokenHyphen 表示连字符 '-'。
	TokenHyphen
	// TokenKeyword 表示关键字（type, service, syntax, info, handler, doc, grpc）。
	TokenKeyword
	// TokenAt 表示 '@'。
	TokenAt
)

// Token 表示一个词法 token。
type Token struct {
	Type  TokenType // token 类型
	Value string    // token 值
	Line  int       // token 所在行号（用于错误报告）
	Col   int       // token 所在列号
}

// String 返回 token 的字符串表示，用于调试。
func (t Token) String() string {
	return fmt.Sprintf("{Type: %v, Value: %q, Line: %d}", t.Type, t.Value, t.Line)
}

// Lexer 是词法分析器。
type Lexer struct {
	input        string          // 输入文本
	position     int             // 当前读取位置
	readPosition int             // 下一个读取位置
	currentRune  rune            // 当前字符
	line         int             // 当前行号
	col          int             // 当前列号
}

// New 创建一个新的词法分析器。
func New(input string) *Lexer {
	l := &Lexer{
		input:   input,
		line:    1,
		col:     0,
		position: 0,
	}
	l.readRune()
	return l
}

// NextToken 返回下一个 token。
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()
	l.skipComments()

	switch l.currentRune {
	case 0:
		return l.makeToken(TokenEOF, "")

	case '(':
		tok := l.makeToken(TokenLParen, string(l.currentRune))
		l.readRune()
		return tok
	case ')':
		tok := l.makeToken(TokenRParen, string(l.currentRune))
		l.readRune()
		return tok
	case '{':
		tok := l.makeToken(TokenLBrace, string(l.currentRune))
		l.readRune()
		return tok
	case '}':
		tok := l.makeToken(TokenRBrace, string(l.currentRune))
		l.readRune()
		return tok
	case '[':
		tok := l.makeToken(TokenLBracket, string(l.currentRune))
		l.readRune()
		return tok
	case ']':
		tok := l.makeToken(TokenRBracket, string(l.currentRune))
		l.readRune()
		return tok
	case '=':
		tok := l.makeToken(TokenAssign, string(l.currentRune))
		l.readRune()
		return tok
	case ',':
		tok := l.makeToken(TokenComma, string(l.currentRune))
		l.readRune()
		return tok
	case ':':
		tok := l.makeToken(TokenColon, string(l.currentRune))
		l.readRune()
		return tok
	case '/':
		tok := l.makeToken(TokenSlash, string(l.currentRune))
		l.readRune()
		return tok
	case '.':
		tok := l.makeToken(TokenDot, string(l.currentRune))
		l.readRune()
		return tok
	case '-':
		tok := l.makeToken(TokenHyphen, string(l.currentRune))
		l.readRune()
		return tok
	case '@':
		tok := l.makeToken(TokenAt, string(l.currentRune))
		l.readRune()
		return tok
	case '"':
		// 字符串字面量
		str, ok := l.readString()
		if !ok {
			return l.makeToken(TokenError, "unclosed string")
		}
		return l.makeToken(TokenString, str)
	}

	if unicode.IsLetter(l.currentRune) || l.currentRune == '_' {
		return l.readIdentifier()
	}

	if unicode.IsDigit(l.currentRune) {
		return l.readNumber()
	}

	// 无法识别的字符
	tok := l.makeToken(TokenError, string(l.currentRune))
	l.readRune()
	return tok
}

// readRune 读取下一个 rune。
func (l *Lexer) readRune() {
	if l.readPosition >= len(l.input) {
		l.currentRune = 0 // EOF
	} else {
		r, w := utf8.DecodeRuneInString(l.input[l.readPosition:])
		l.currentRune = r
		l.position = l.readPosition
		l.readPosition += w
		if r == '\n' {
			l.line++
			l.col = 0
		} else {
			l.col++
		}
	}
}

// skipWhitespace 跳过连续的空白字符。
func (l *Lexer) skipWhitespace() {
	for unicode.IsSpace(l.currentRune) {
		l.readRune()
	}
}

// skipComments 跳过 // 注释。
func (l *Lexer) skipComments() {
	if l.currentRune == '/' && l.peek() == '/' {
		// 跳过整个注释行，直到换行或 EOF
		for l.currentRune != '\n' && l.currentRune != 0 {
			l.readRune()
		}
		l.skipWhitespace()
	}
}

// peek 读取下一个字符，但不移动位置。
func (l *Lexer) peek() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition:])
	return r
}

// readIdentifier 读取标识符。
func (l *Lexer) readIdentifier() Token {
	start := l.position
	for unicode.IsLetter(l.currentRune) || unicode.IsDigit(l.currentRune) || l.currentRune == '_' {
		l.readRune()
	}
	value := l.input[start:l.position]
	return l.makeToken(identOrKeyword(value), value)
}

// readNumber 读取数字。
func (l *Lexer) readNumber() Token {
	start := l.position
	for unicode.IsDigit(l.currentRune) {
		l.readRune()
	}
	// 支持小数
	if l.currentRune == '.' && unicode.IsDigit(l.peek()) {
		l.readRune()
		for unicode.IsDigit(l.currentRune) {
			l.readRune()
		}
	}
	value := l.input[start:l.position]
	return l.makeToken(TokenNumber, value)
}

// readString 读取字符串字面量。
func (l *Lexer) readString() (string, bool) {
	l.readRune() // 跳过开头的 "
	start := l.position
	for l.currentRune != '"' && l.currentRune != 0 {
		if l.currentRune == '\\' {
			l.readRune() // 跳过转义字符
		}
		l.readRune()
	}
	if l.currentRune == 0 {
		return "", false
	}
	value := l.input[start:l.position]
	l.readRune() // 跳过结尾的 "
	return value, true
}

// identOrKeyword 判断标识符是否为关键字。
func identOrKeyword(s string) TokenType {
	switch s {
	case "syntax", "info", "type", "server", "service", "handler", "doc", "grpc", "returns":
		return TokenKeyword
	default:
		return TokenIdent
	}
}

// makeToken 创建 token。
func (l *Lexer) makeToken(t TokenType, v string) Token {
	return Token{
		Type:  t,
		Value: v,
		Line:  l.line,
		Col:   l.col - len(v),
	}
}

// IsKeyword 检查字符串是否为关键字。
func IsKeyword(s string) bool {
	switch s {
	case "syntax", "info", "type", "server", "service", "handler", "doc", "grpc", "returns":
		return true
	default:
		return false
	}
}