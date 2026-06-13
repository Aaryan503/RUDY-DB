package main

import (
	"fmt"
	"strings"
)

type Statement interface {
	statementNode()
}

type WhereClause struct {
	Conditions []Condition
}

type SelectStatement struct {
	TableName string
	Where     *WhereClause
	Star      bool
	Fields    []string
}

func (s *SelectStatement) statementNode() {}

type CreateTableStatement struct {
	TableName string
	Columns   []Column
}

func (c *CreateTableStatement) statementNode() {}

type InsertStatement struct {
	TableName string
	Values    []any
}

func (i *InsertStatement) statementNode() {}

type DeleteStatement struct {
	TableName string
	Where     *WhereClause
}

func (d *DeleteStatement) statementNode() {}

type DropStatement struct {
	TableName string
}

func (d *DropStatement) statementNode() {}

type Parser struct {
	l         *Lexer
	curToken  Token
	peekToken Token
}

func newParser(l *Lexer) *Parser {
	p := &Parser{l: l}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.nextToken()
}

func (p *Parser) parseStatement() (Statement, error) {
	if p.curToken.Type == TokenKeyword {
		switch strings.ToUpper(p.curToken.Value) {
		case "SELECT":
			return p.parseSelectStatement()
		case "CREATE":
			return p.parseCreateTableStatement()
		case "INSERT":
			return p.parseInsertStatement()
		case "DELETE":
			return p.parseDeleteStatement()
		case "DROP":
			return p.parseDropStatement()
		}
	}
	return nil, fmt.Errorf("unsupported statement: %s", p.curToken.Value)
}

func (p *Parser) parseWhere() (*WhereClause, error) {
	where := &WhereClause{}
	for {
		if p.curToken.Type != TokenIdentifier {
			return nil, fmt.Errorf("expected field name for the WHERE clause but got %s", p.curToken.Value)
		}
		field := p.curToken.Value
		p.nextToken()
		op := p.curToken.Value
		if op != "=" && op != "!=" && op != "<" && op != ">" && op != "<=" && op != ">=" {
			return nil, fmt.Errorf("expected operator, got %s", op)
		}
		p.nextToken()
		if p.curToken.Type != TokenIdentifier {
			return nil, fmt.Errorf("expected an identifier in WHERE after operator got %s", p.curToken.Value)
		}
		where.Conditions = append(where.Conditions, Condition{field, op, p.curToken.Value})
		if strings.ToUpper(p.curToken.Value) == "AND" {
			p.nextToken()
			continue
		}
		break
	}
	return where, nil
}

func (p *Parser) parseSelectStatement() (*SelectStatement, error) {
	stmt := &SelectStatement{}
	p.nextToken()
	if p.curToken.Type == TokenSymbol && p.curToken.Value == "*" {
		stmt.Fields = nil
		stmt.Star = true
		p.nextToken()
	} else {
		for p.curToken.Type == TokenIdentifier {
			stmt.Fields = append(stmt.Fields, p.curToken.Value)
			p.nextToken()
			if p.curToken.Type == TokenSymbol && p.curToken.Value == "," {
				p.nextToken()
			}
		}
	}
	if strings.ToUpper(p.curToken.Value) != "FROM" {
		return nil, fmt.Errorf("expected FROM, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) == "WHERE" {
		p.nextToken()
		where, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}
	if p.curToken.Type == TokenSymbol && p.curToken.Value == ";" {
		p.nextToken()
	}
	return stmt, nil
}

func (p *Parser) parseCreateTableStatement() (*CreateTableStatement, error) {
	stmt := &CreateTableStatement{}
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) != "TABLE" {
		return nil, fmt.Errorf("expected TABLE, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	p.nextToken()
	if p.curToken.Type == TokenSymbol && p.curToken.Value == "(" {
		p.nextToken()
		for p.curToken.Type == TokenIdentifier {
			colName := p.curToken.Value
			p.nextToken()
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expected column type, got %s", p.curToken.Value)
			}
			colType := p.curToken.Value
			p.nextToken()
			stmt.Columns = append(stmt.Columns, Column{Name: colName, Type: colType})
			if p.curToken.Type == TokenSymbol && p.curToken.Value == "," {
				p.nextToken()
			}
		}
		if p.curToken.Type != TokenSymbol || p.curToken.Value != ")" {
			return nil, fmt.Errorf("expected ), got %s", p.curToken.Value)
		}
		p.nextToken()
	}
	if p.curToken.Type == TokenSymbol && p.curToken.Value == ";" {
		p.nextToken()
	}
	return stmt, nil
}

func (p *Parser) parseInsertStatement() (*InsertStatement, error) {
	stmt := &InsertStatement{}
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) != "INTO" {
		return nil, fmt.Errorf("expected INTO, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) != "VALUES" {
		return nil, fmt.Errorf("expected VALUES, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type == TokenSymbol && p.curToken.Value == "(" {
		p.nextToken()
		for p.curToken.Type != TokenSymbol || p.curToken.Value != ")" {
			if p.curToken.Type == TokenIdentifier {
				stmt.Values = append(stmt.Values, p.curToken.Value)
			} else {
				return nil, fmt.Errorf("expected value, got %s", p.curToken.Value)
			}
			p.nextToken()
			if p.curToken.Type == TokenSymbol && p.curToken.Value == "," {
				p.nextToken()
			}
		}
		p.nextToken()
	}
	return stmt, nil
}

func (p *Parser) parseDeleteStatement() (*DeleteStatement, error) {
	stmt := &DeleteStatement{}
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) != "FROM" {
		return nil, fmt.Errorf("expected FROM, got %s ", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expect table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) == "WHERE" {
		p.nextToken()
		where, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}
	return stmt, nil
}

func (p *Parser) parseDropStatement() (*DropStatement, error) {
	stmt := &DropStatement{}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	return stmt, nil
}
