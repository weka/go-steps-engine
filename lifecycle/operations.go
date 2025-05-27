package lifecycle

import "context"

type Operation interface {
	AsStep() Step
	GetSteps() []Step
	GetJsonResult() string
}

func ExecuteOperation(ctx context.Context, op Operation) error {
	step := op.AsStep()
	stepsEngine := StepsEngine{
		Steps: []Step{step},
	}
	return stepsEngine.Run(ctx)
}

func AsRunFunc(op Operation) StepFunc {
	return func(ctx context.Context) error {
		steps := op.GetSteps()
		reconSteps := StepsEngine{
			Steps: steps,
		}
		return reconSteps.Run(ctx)
	}
}
