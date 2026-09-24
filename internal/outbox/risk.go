package outbox

import "github.com/jiying2007/engineering-platform/internal/action"

func ValidRisk(risk action.RiskClass) bool {
	return risk == action.Observe || risk == action.ControlledMutation || risk == action.HighRisk
}

// Classify is the trusted Core topic registry. Unknown topics require an
// explicit valid classification; known Core topics cannot be downgraded.
func Classify(topic string, declared action.RiskClass) (action.RiskClass, error) {
	var required action.RiskClass
	switch topic {
	case "run.started":
		required = action.ControlledMutation
	case "work.created":
		required = action.Observe
	}
	if required != "" {
		if declared != "" && declared != required {
			return "", ErrRiskClass
		}
		return required, nil
	}
	if topic == "" || !ValidRisk(declared) {
		return "", ErrRiskClass
	}
	return declared, nil
}
