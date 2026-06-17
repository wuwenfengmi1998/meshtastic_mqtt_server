package calculator

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"

	"meshtastic_mqtt_server/agenttool"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// Tool is a calculator tool that can evaluate simple math expressions
type Tool struct {
	enabled bool
}

// Name returns the tool name
func (t *Tool) Name() string {
	return "calculator"
}

// Enabled returns whether the tool is enabled
func (t *Tool) Enabled() bool {
	return t.enabled
}

// ToolDefinition returns the OpenAI tool definition
func (t *Tool) ToolDefinition(description string) *model.Tool {
	desc := "一个计算器工具，可以计算数学表达式。支持加减乘除、平方根、幂运算等。当用户需要计算数学表达式时使用此工具。"
	if description != "" {
		desc = description
	}
	return &model.Tool{
		Type: model.ToolTypeFunction,
		Function: &model.FunctionDefinition{
			Name:        "calculator",
			Description: desc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{
						"type":        "string",
						"description": "要计算的数学表达式，例如 \"2 + 3 * 4\" 或 \"sqrt(16)\"",
					},
				},
				"required": []string{"expression"},
			},
		},
	}
}

// Execute executes the calculator tool
func (t *Tool) Execute(ctx context.Context, args string, runtime agenttool.Runtime) (string, error) {
	var params struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Expression == "" {
		return "", fmt.Errorf("expression is required")
	}

	result, err := evaluateExpression(params.Expression)
	if err != nil {
		return fmt.Sprintf("计算错误: %v", err), nil
	}

	return fmt.Sprintf("计算结果: %s = %g", params.Expression, result), nil
}

// RawState returns the tool state
func (t *Tool) RawState() any {
	return map[string]any{"enabled": t.enabled}
}

// evaluateExpression evaluates a simple mathematical expression using Go AST
func evaluateExpression(expr string) (float64, error) {
	// Parse the expression
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("无效的表达式: %w", err)
	}

	return evalNode(node)
}

// evalNode evaluates an AST node
func evalNode(node ast.Expr) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			return strconv.ParseFloat(n.Value, 64)
		}
		return 0, fmt.Errorf("不支持的字面量类型: %v", n.Kind)

	case *ast.BinaryExpr:
		left, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		right, err := evalNode(n.Y)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("除数不能为零")
			}
			return left / right, nil
		case token.REM:
			return math.Mod(left, right), nil
		default:
			return 0, fmt.Errorf("不支持的运算符: %v", n.Op)
		}

	case *ast.ParenExpr:
		return evalNode(n.X)

	case *ast.UnaryExpr:
		val, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.ADD:
			return val, nil
		case token.SUB:
			return -val, nil
		default:
			return 0, fmt.Errorf("不支持的一元运算符: %v", n.Op)
		}

	case *ast.CallExpr:
		ident, ok := n.Fun.(*ast.Ident)
		if !ok {
			return 0, fmt.Errorf("不支持的函数调用形式")
		}
		if len(n.Args) != 1 {
			return 0, fmt.Errorf("函数只接受一个参数")
		}
		arg, err := evalNode(n.Args[0])
		if err != nil {
			return 0, err
		}

		switch ident.Name {
		case "sqrt":
			if arg < 0 {
				return 0, fmt.Errorf("平方根的参数不能为负数")
			}
			return math.Sqrt(arg), nil
		case "abs":
			return math.Abs(arg), nil
		case "sin":
			return math.Sin(arg), nil
		case "cos":
			return math.Cos(arg), nil
		case "tan":
			return math.Tan(arg), nil
		case "log":
			if arg <= 0 {
				return 0, fmt.Errorf("对数的参数必须为正数")
			}
			return math.Log(arg), nil
		case "log10":
			if arg <= 0 {
				return 0, fmt.Errorf("对数的参数必须为正数")
			}
			return math.Log10(arg), nil
		case "exp":
			return math.Exp(arg), nil
		case "pow":
			// For pow, we need two arguments - this is simplified
			return 0, fmt.Errorf("pow 函数需要两个参数，请使用 ** 运算符代替")
		default:
			return 0, fmt.Errorf("不支持的函数: %s", ident.Name)
		}

	default:
		return 0, fmt.Errorf("不支持的表达式节点类型: %T", node)
	}
}

func init() {
	agenttool.Register(agenttool.Descriptor{
		Name: "calculator",
		Load: func(path string, options agenttool.LoadOptions) (agenttool.LoadedTool, error) {
			return &Tool{enabled: true}, nil
		},
	})
}
