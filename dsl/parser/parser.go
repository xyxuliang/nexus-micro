// Package parser 提供 .api DSL 的语法分析器。
// 将 Lexer 输出的 token 流解析为 AST（抽象语法树）。
// 支持递归下降解析，语法兼容 go-zero 的 .api 格式并增强 @grpc 注解。
package parser

import (
	"fmt"
	"strings"

	"github.com/xyxuliang/nexus-micro/dsl/ast"
	"github.com/xyxuliang/nexus-micro/dsl/lexer"
)

// Parser 是语法分析器。
type Parser struct {
	lexer        *lexer.Lexer // 词法分析器
	currentToken lexer.Token  // 当前 token
	peekToken    lexer.Token  // 下一个 token（lookahead）
	file         *ast.File    // 当前正在解析的 AST
	errors       []error      // 错误列表
}

// New 创建一个新的语法分析器。
func New(input string) *Parser {
	p := &Parser{
		lexer: lexer.New(input),
		file:  &ast.File{},
	}

	// 预读两个 token（current + peek）
	p.nextToken()
	p.nextToken()
	return p
}

// Parse 解析 .api 文件，返回 AST 和解析过程中遇到的错误列表。
func (p *Parser) Parse() (*ast.File, []error) {
	p.parseFile()
	return p.file, p.errors
}

// nextToken 读取下一个 token（推进 currentToken）。
func (p *Parser) nextToken() {
	p.currentToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

// currentTokenIs 检查当前 token 是否为指定类型。
func (p *Parser) currentTokenIs(t lexer.TokenType) bool {
	return p.currentToken.Type == t
}

// peekTokenIs 检查下一个 token 是否为指定类型。
func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

// expectPeek 检查下一个 token 是否为指定类型，如果是则推进。
func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.addError(fmt.Sprintf("line %d: expected %v but got %v (%q)", p.peekToken.Line, t, p.peekToken.Type, p.peekToken.Value))
	return false
}

// parseFile 解析整个文件。
func (p *Parser) parseFile() {
	for !p.currentTokenIs(lexer.TokenEOF) {
		switch {
		case p.currentTokenIs(lexer.TokenKeyword) && p.currentToken.Value == "syntax":
			p.parseSyntax()
		case p.currentTokenIs(lexer.TokenKeyword) && p.currentToken.Value == "info":
			p.parseInfo()
		case p.currentTokenIs(lexer.TokenKeyword) && p.currentToken.Value == "type":
			p.parseTypeBlock()
		case p.currentTokenIs(lexer.TokenAt):
			p.parseServiceBlock()
		default:
			p.nextToken()
		}
	}
}

// parseSyntax 解析 syntax = "v1" 语句。
func (p *Parser) parseSyntax() {
	if !p.expectPeek(lexer.TokenAssign) {
		return
	}
	p.nextToken()
	if p.currentTokenIs(lexer.TokenString) {
		p.file.Syntax = p.currentToken.Value
	}
	p.nextToken()
}

// parseInfo 解析 info(...) 块。
func (p *Parser) parseInfo() {
	if !p.expectPeek(lexer.TokenLParen) {
		return
	}

	info := &ast.Info{}
	for !p.currentTokenIs(lexer.TokenRParen) && !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenIdent) {
			key := p.currentToken.Value
			p.nextToken()
			if p.currentTokenIs(lexer.TokenColon) {
				p.nextToken()
				if p.currentTokenIs(lexer.TokenString) {
					switch key {
					case "title":
						info.Title = p.currentToken.Value
					case "desc":
						info.Desc = p.currentToken.Value
					case "version":
						info.Version = p.currentToken.Value
					case "author":
						info.Author = p.currentToken.Value
					}
				}
			}
		}
		p.nextToken()
	}
	p.file.Info = info
	p.nextToken() // 跳过 ')'
}

// parseTypeBlock 解析 type(...) 块。
func (p *Parser) parseTypeBlock() {
	if !p.expectPeek(lexer.TokenLParen) {
		return
	}
	p.nextToken()

	for !p.currentTokenIs(lexer.TokenRParen) && !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenIdent) {
			t := p.parseType()
			if t != nil {
				p.file.Types = append(p.file.Types, t)
			}
		} else {
			p.nextToken()
		}
	}
	p.nextToken() // 跳过 ')'
}

// parseType 解析单个类型定义。
func (p *Parser) parseType() *ast.Type {
	name := p.currentToken.Value
	t := &ast.Type{Name: name}

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken()

	// 解析字段
	for !p.currentTokenIs(lexer.TokenRBrace) && !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenIdent) {
			field := p.parseField()
			if field != nil {
				t.Fields = append(t.Fields, field)
			}
		} else {
			p.nextToken()
		}
	}
	p.nextToken() // 跳过 '}'
	return t
}

// parseField 解析单个字段定义。
func (p *Parser) parseField() *ast.Field {
	field := &ast.Field{
		Name: p.currentToken.Value,
	}

	p.nextToken()
	if !p.currentTokenIs(lexer.TokenIdent) {
		return nil
	}
	field.Type = p.currentToken.Value

	// 解析反引号内的标签
	p.nextToken()
	if p.currentTokenIs(lexer.TokenString) {
		// 可能的标签字符串
	}
	// 简化处理：跳过标签解析
	if p.currentToken.Value == "`" {
		p.nextToken()
	}

	return field
}

// parseServiceBlock 解析 @server(...) 和 service XxxService {...} 块。
func (p *Parser) parseServiceBlock() {
	p.nextToken() // 跳过 '@'

	if !p.currentTokenIs(lexer.TokenKeyword) || p.currentToken.Value != "server" {
		return
	}

	// 解析 @server 注解
	cfg := p.parseServerConfig()

	// 等待 service 关键字
	for !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenKeyword) && p.currentToken.Value == "service" {
			p.parseService(cfg)
			return
		}
		p.nextToken()
	}
}

// parseServerConfig 解析 @server(...) 配置。
func (p *Parser) parseServerConfig() *ast.Config {
	cfg := &ast.Config{}

	if !p.expectPeek(lexer.TokenLParen) {
		return cfg
	}

	p.nextToken()
	for !p.currentTokenIs(lexer.TokenRParen) && !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenIdent) || p.currentTokenIs(lexer.TokenKeyword) {
			key := p.currentToken.Value
			p.nextToken()
			if p.currentTokenIs(lexer.TokenColon) {
				p.nextToken()
				switch key {
				case "prefix":
					if p.currentTokenIs(lexer.TokenString) {
						cfg.Prefix = p.currentToken.Value
					}
				case "service":
					if p.currentTokenIs(lexer.TokenIdent) {
						cfg.Service = p.currentToken.Value
					}
				case "middleware":
					if p.currentTokenIs(lexer.TokenIdent) {
						cfg.Middleware = parseCommaSeparated(p.currentToken.Value)
					}
				}
			}
		}
		p.nextToken()
	}
	p.nextToken() // 跳过 ')'
	return cfg
}

// parseService 解析 service XxxService {...} 块。
func (p *Parser) parseService(cfg *ast.Config) {
	n := p.currentToken.Value // 'service'
	_ = n

	p.nextToken()
	if !p.currentTokenIs(lexer.TokenIdent) {
		return
	}

	svc := &ast.Service{
		Name:       p.currentToken.Value,
		Prefix:     cfg.Prefix,
		Group:      cfg.Service,
		Middleware: cfg.Middleware,
	}

	if !p.expectPeek(lexer.TokenLBrace) {
		return
	}

	p.nextToken()
	for !p.currentTokenIs(lexer.TokenRBrace) && !p.currentTokenIs(lexer.TokenEOF) {
		if p.currentTokenIs(lexer.TokenAt) {
			handler := p.parseHandler()
			if handler != nil {
				svc.Handlers = append(svc.Handlers, handler)
			}
		} else {
			p.nextToken()
		}
	}

	p.file.Services = append(p.file.Services, svc)
	p.nextToken() // 跳过 '}'
}

// parseHandler 解析 @handler + @doc + @grpc + route 定义。
func (p *Parser) parseHandler() *ast.Handler {
	handler := &ast.Handler{}

	// 解析注解
	for p.currentTokenIs(lexer.TokenAt) {
		p.nextToken()
		if p.currentTokenIs(lexer.TokenKeyword) {
			switch p.currentToken.Value {
			case "handler":
				p.nextToken()
				if p.currentTokenIs(lexer.TokenIdent) {
					handler.Name = p.currentToken.Value
				}
			case "doc":
				p.nextToken()
				if p.currentTokenIs(lexer.TokenString) {
					handler.Doc = p.currentToken.Value
				}
			case "grpc":
				handler.HasGRPC = true
			}
		}
		p.nextToken()
	}

	// 解析 HTTP 方法
	if p.currentTokenIs(lexer.TokenIdent) {
		handler.Method = strings.ToUpper(p.currentToken.Value)
	}

	// 解析路由路径
	p.nextToken()
	if p.currentTokenIs(lexer.TokenSlash) {
		p.nextToken()
		path := "/" + p.currentToken.Value
		handler.Path = path
	}

	return handler
}

// parseCommaSeparated 解析逗号分隔的字符串列表。
func parseCommaSeparated(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// addError 添加解析错误。
func (p *Parser) addError(msg string) {
	p.errors = append(p.errors, fmt.Errorf("parser: %s", msg))
}
