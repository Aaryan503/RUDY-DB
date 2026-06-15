package main

import (
	"fmt"
	"strconv"
	"strings"
)

type Statement interface {
	statementNode()
}

type Aggregate struct {
	Keyword string
	Field   string
	Alias   string
}

type SelectStatement struct {
	TableName  string
	Where      *WhereClause
	Star       bool
	Fields     []string
	Limit      int
	Distinct   bool
	Aggregates []Aggregate
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

func (u *UpdateStatement) statementNode() {}

type UpdateStatement struct {
	TableName string
	Updates   map[string]string
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
		case "UPDATE":
			return p.parseUpdateStatement()
		}
	}
	return nil, fmt.Errorf("unsupported statement: %s", p.curToken.Value)
}

func (p *Parser) parseWhere() (*WhereClause, error) {
	root, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	return &WhereClause{root: root}, nil
}

func (p *Parser) parseOrExpr() (*ExprNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}
	for strings.ToUpper(p.curToken.Value) == "OR" {
		p.nextToken()
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &ExprNode{
			Op:    "OR",
			Left:  left,
			Right: right,
		}
	}
	return left, nil
}

func (p *Parser) parseAndExpr() (*ExprNode, error) {
	left, err := p.parseCondition()
	if err != nil {
		return nil, err
	}
	for strings.ToUpper(p.curToken.Value) == "AND" {
		p.nextToken()
		right, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		left = &ExprNode{
			Op:    "AND",
			Left:  left,
			Right: right,
		}
	}
	return left, nil
}

func (p *Parser) parseCondition() (*ExprNode, error) {
	if p.curToken.Type == TokenSymbol && p.curToken.Value == "(" {
		p.nextToken()
		node, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		if p.curToken.Value != ")" {
			return nil, fmt.Errorf("expected closing ), got %s", p.curToken.Value)
		}
		p.nextToken()
		return node, nil
	}
	if strings.ToUpper(p.curToken.Value) == "NOT" {
		p.nextToken()
		left, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		left = &ExprNode{
			Op:   "NOT",
			Left: left,
		}
		return left, nil
	}

	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected field name in WHERE, got %s", p.curToken.Value)
	}
	field := p.curToken.Value
	p.nextToken()
	op := p.curToken.Value
	if op != "=" && op != "!=" && op != "<" && op != ">" && op != "<=" && op != ">=" {
		return nil, fmt.Errorf("expected operator, got %s", op)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected value after operator, got %s", p.curToken.Value)
	}
	cond := &Condition{
		Operator: op,
		Field:    field,
		Value:    p.curToken.Value,
	}
	p.nextToken()
	return &ExprNode{Condition: cond}, nil
}

func (p *Parser) parseSelectStatement() (*SelectStatement, error) {
	stmt := &SelectStatement{}
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) == "DISTINCT" {
		stmt.Distinct = true
		p.nextToken()
	} else {
		stmt.Distinct = false
	}
	if p.curToken.Type == TokenSymbol && p.curToken.Value == "*" {
		stmt.Fields = nil
		stmt.Star = true
		p.nextToken()
	} else {
		for p.curToken.Type == TokenKeyword {
			aggregateFunc := strings.ToUpper(p.curToken.Value)
			if aggregateFunc != "MAX" && aggregateFunc != "MIN" && aggregateFunc != "AVG" && aggregateFunc != "SUM" && aggregateFunc != "COUNT" {
				break
			}
			p.nextToken()
			if p.curToken.Value != "(" {
				return nil, fmt.Errorf("expected ( after aggregate function, got %s", p.curToken.Value)
			}
			p.nextToken()
			var field string
			if aggregateFunc == "COUNT" && p.curToken.Value == "*" {
				field = p.curToken.Value
			} else if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("Expected field after aggregator function %s, got %s instead", aggregateFunc, p.curToken.Value)
			} else {
				field = p.curToken.Value
			}
			p.nextToken()
			if p.curToken.Value != ")" {
				return nil, fmt.Errorf("expected ) after %s field, got %s instead", field, p.curToken.Value)
			}
			p.nextToken()
			agg := Aggregate{aggregateFunc, field, ""}
			if strings.ToUpper(p.curToken.Value) == "AS" {
				p.nextToken()
				agg.Alias = p.curToken.Value
				p.nextToken()
			}
			stmt.Aggregates = append(stmt.Aggregates, agg)
			if p.curToken.Value == "," {
				p.nextToken()
				continue
			}
			break
		}
		if len(stmt.Aggregates) == 0 {
			for p.curToken.Type == TokenIdentifier {
				stmt.Fields = append(stmt.Fields, p.curToken.Value)
				p.nextToken()
				if p.curToken.Type == TokenSymbol && p.curToken.Value == "," {
					p.nextToken()
				}
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
	if strings.ToUpper(p.curToken.Value) == "LIMIT" {
		p.nextToken()
		val := p.curToken.Value
		limit, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("expected a numerical limit, got %s", p.curToken.Value)
		}
		stmt.Limit = limit
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

func (p *Parser) parseUpdateStatement() (*UpdateStatement, error) {
	stmt := &UpdateStatement{
		Updates: make(map[string]string),
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	p.nextToken()
	if strings.ToUpper(p.curToken.Value) != "SET" {
		return nil, fmt.Errorf("expected SET, got %s", p.curToken.Value)
	}
	p.nextToken()
	for p.curToken.Type == TokenIdentifier {
		colName := p.curToken.Value
		p.nextToken()

		if p.curToken.Value != "=" {
			return nil, fmt.Errorf("expected = after column name, got %s", p.curToken.Value)
		}
		p.nextToken()

		if p.curToken.Type != TokenIdentifier {
			return nil, fmt.Errorf("expected value, got %s", p.curToken.Value)
		}
		stmt.Updates[colName] = p.curToken.Value
		p.nextToken()
		if p.curToken.Type == TokenSymbol && p.curToken.Value == "," {
			p.nextToken()
			continue
		}
		break
	}
	if len(stmt.Updates) == 0 {
		return nil, fmt.Errorf("UPDATE requires at least one column assignment")
	}
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
	if strings.ToUpper(p.curToken.Value) != "TABLE" {
		return nil, fmt.Errorf("expected TABLE before table name, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expected table name, got %s", p.curToken.Value)
	}
	stmt.TableName = p.curToken.Value
	return stmt, nil
}
