package acceptance

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Result struct {
	Passed bool
	Reason string
}

type EvalParams struct {
	MinObservations            int
	RequiredImprovement        float64
	WelchTTestThreshold        float64
	DrawdownProtectionRatio    float64
	VolatilityToleranceRatio   float64
	MaxFallbackRatio           float64
	FactorWeightDriftThreshold float64
	SharpeStabilityThreshold   float64
	PromptBytes                []byte
}

type Evaluator interface {
	Name() string
	Eval(input domain.PromptExperimentResult, params EvalParams) Result
}

type EvalFunc func(input domain.PromptExperimentResult, params EvalParams) Result

type FuncEvaluator struct {
	N string
	F EvalFunc
}

func (fe FuncEvaluator) Name() string { return fe.N }
func (fe FuncEvaluator) Eval(input domain.PromptExperimentResult, params EvalParams) Result {
	return fe.F(input, params)
}

type Pipeline struct {
	evaluators []Evaluator
}

func NewPipeline(evaluators ...Evaluator) *Pipeline {
	return &Pipeline{evaluators: evaluators}
}

func (p *Pipeline) Run(input domain.PromptExperimentResult, params EvalParams) (bool, string) {
	for _, e := range p.evaluators {
		r := e.Eval(input, params)
		if !r.Passed {
			return false, fmt.Sprintf("%s: %s", e.Name(), r.Reason)
		}
	}
	return true, "accepted: all acceptance evaluators passed"
}

type Registry struct {
	evaluators map[string]Evaluator
}

func NewRegistry() *Registry {
	return &Registry{evaluators: make(map[string]Evaluator)}
}

func (r *Registry) Register(e Evaluator) {
	r.evaluators[e.Name()] = e
}

func (r *Registry) Get(name string) (Evaluator, bool) {
	e, ok := r.evaluators[name]
	return e, ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.evaluators))
	for n := range r.evaluators {
		names = append(names, n)
	}
	return names
}
