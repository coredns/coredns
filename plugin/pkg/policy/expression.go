package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/coredns/request"

	"github.com/coredns/coredns/plugin/metadata"

	expr "github.com/Knetic/govaluate"
)

type policyRuleExpression struct {
	kind        byte
	kindIfError byte
	expression  *expr.EvaluableExpression
}

// ExpressionEngine implement interface Engine for Firewall plugin
// it evaluate the rues using an the lib Knetic/govaluate
type ExpressionEngine struct {
	kindIfErrorEvaluation byte
	dataFromReq           *RequestExtractorMapping
}

type dataAsParam struct {
	ctx         context.Context
	dataFromReq *RequestDataExtractor
}

// NewExpressionEngine create a new Engine with default configuration
func NewExpressionEngine() *ExpressionEngine {
	return &ExpressionEngine{TypeRefuse, NewRequestExtractorMapping("")}
}

//BuildQueryData here return a dataAsParam that can be used by to evaluate the variables of the expression
func (x *ExpressionEngine) BuildQueryData(ctx context.Context, state request.Request) (interface{}, error) {
	return &dataAsParam{ctx, &RequestDataExtractor{state, x.dataFromReq}}, nil
}

//BuildReplyData here return a dataAsParam that can be used by to evaluate the variables of the expression
func (x *ExpressionEngine) BuildReplyData(ctx context.Context, state request.Request, query interface{}) (interface{}, error) {
	return &dataAsParam{ctx, &RequestDataExtractor{state, x.dataFromReq}}, nil
}

//BuildRule create a rule for Expression Engine:
// - first param is one of the action to return
// - second and following param is a sentence the represent an Expression
func (x *ExpressionEngine) BuildRule(args []string) (Rule, error) {
	keyword := args[0]
	exp := args[1:]
	e, err := expr.NewEvaluableExpression(strings.Join(exp, " "))
	if err != nil {
		return nil, fmt.Errorf("cannot create a valid expression : %s", err)
	}

	var kind = byte(TypeNone)
	for k, n := range NameTypes {
		if keyword == n {
			kind = byte(k)
		}
	}
	if kind == TypeNone {
		return nil, fmt.Errorf("invalid keyword %s for a policy rule", keyword)
	}
	return &policyRuleExpression{kind, x.kindIfErrorEvaluation, e}, nil
}

func toBoolean(v interface{}) (bool, error) {
	if s, ok := v.(string); ok {
		return strings.ToLower(s) == "true", nil
	}
	if b, ok := v.(bool); ok {
		return b, nil
	}
	if i, ok := v.(int); ok {
		return i != 0, nil
	}
	return false, fmt.Errorf("cannot extract a boolean value from result of expression")
}

//Evaluate the current expression, using data as a variable resolver for Expression
func (r *policyRuleExpression) Evaluate(data interface{}) (byte, error) {

	params, ok := data.(*dataAsParam)
	if !ok {
		return r.kindIfError, fmt.Errorf("evaluation of expression '%s' - params provided are of wrong type, expect a go Context", r.expression.String())
	}
	res, err := r.expression.Eval(params)
	if err != nil {
		return r.kindIfError, fmt.Errorf("evaluation of expression '%s' return an error : %s", r.expression.String(), err)
	}
	result, err := toBoolean(res)
	if err != nil {
		return r.kindIfError, fmt.Errorf("evaluation of expression '%s' return an non boolean value : %s", r.expression.String(), err)
	}

	if result {
		return r.kind, nil
	}
	return TypeNone, nil
}

// Get return the value associated with the variable
// DataRequestExtractor is evaluated first, and if the name does not match then metadata is evaluated
func (p *dataAsParam) Get(name string) (interface{}, error) {
	v, exist := p.dataFromReq.Value(name)
	if exist {
		return v, nil
	}
	f := metadata.ValueFunc(p.ctx, name)
	if f == nil {
		return "", nil
	}
	return f(), nil
}
