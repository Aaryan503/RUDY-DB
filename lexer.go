package main

import (
	"strings"
)

type TokenType string

const (
	TokenKeyword    TokenType = "KEYWORD"
	TokenIdentifier TokenType = "IDENTIFIER"
	TokenSymbol     TokenType = "SYMBOL"
	TokenEOF        TokenType = "EOF"
)

type Token struct {
	Type  TokenType
	Value string
}

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
}

func newLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) nextToken() Token {
	var tok Token
	l.skipWhitespace()
	switch l.ch {
	case ',':
		tok = Token{Type: TokenSymbol, Value: ","}
	case ';':
		tok = Token{Type: TokenSymbol, Value: ";"}
	case 0:
		tok = Token{Type: TokenEOF, Value: ""}
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenSymbol, Value: "="}
		} else {
			tok = Token{Type: TokenSymbol, Value: "="}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenSymbol, Value: "!="}
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenSymbol, Value: "<="}
		} else {
			tok = Token{Type: TokenSymbol, Value: "<"}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenSymbol, Value: ">="}
		} else {
			tok = Token{Type: TokenSymbol, Value: ">"}
		}
	case '-':
		if isDigit(l.peekChar()) {
			tok.Type = TokenIdentifier
			tok.Value = l.readNumber()
			return tok
		}
		tok = Token{Type: TokenSymbol, Value: "-"}
	case '+':
		if isDigit(l.peekChar()) {
			tok.Type = TokenIdentifier
			tok.Value = l.readNumber()
			return tok
		}
		tok = Token{Type: TokenSymbol, Value: "+"}
	default:
		if isLetter(l.ch) {
			tok.Value = l.readIdentifier()
			tok.Type = lookupIdentifier(tok.Value)
			return tok
		} else if isDigit(l.ch) {
			tok.Type = TokenIdentifier
			tok.Value = l.readNumber()
			return tok
		} else if l.ch == '"' || l.ch == '\'' {
			tok.Type = TokenIdentifier
			tok.Value = l.readString(l.ch)
			return tok
		}
		tok = Token{Type: TokenSymbol, Value: string(l.ch)}
	}
	l.readChar()
	return tok
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	if l.ch == '-' || l.ch == '+' {
		l.readChar()
	}
	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString(quote byte) string {
	position := l.position + 1
	l.readChar()
	for l.ch != quote && l.ch != 0 {
		l.readChar()
	}
	str := l.input[position:l.position]
	l.readChar()
	return str
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func lookupIdentifier(ident string) TokenType {
	keywords := map[string]bool{
		"SELECT": true, "FROM": true, "CREATE": true, "TABLE": true, "INSERT": true, "INTO": true, "VALUES": true,
		"WHERE": true, "DELETE": true, "DROP": true, "AND": true, "OR": true, "NOT": true, "UPDATE": true, "SET": true,
		"LIMIT": true, "DISTINCT": true, "SUM": true, "AVG": true, "COUNT": true, "MIN": true, "MAX": true,
	}
	if keywords[strings.ToUpper(ident)] {
		return TokenKeyword
	}
	return TokenIdentifier
}
