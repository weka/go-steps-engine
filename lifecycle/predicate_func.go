package lifecycle

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PredicateFunc func() bool
type Predicates []PredicateFunc

func IsNotTrueCondition(condition string, currentConditions *[]metav1.Condition) PredicateFunc {
	return func() bool {
		return !meta.IsStatusConditionTrue(*currentConditions, condition)
	}
}

func IsTrueCondition(condition string, currentConditions *[]metav1.Condition) PredicateFunc {
	return func() bool {
		return meta.IsStatusConditionTrue(*currentConditions, condition)
	}
}

func Or(predicates ...PredicateFunc) PredicateFunc {
	return func() bool {
		for _, predicate := range predicates {
			if predicate() {
				return true
			}
		}
		return false
	}
}

func And(predicates ...PredicateFunc) PredicateFunc {
	return func() bool {
		for _, predicate := range predicates {
			if !predicate() {
				return false
			}
		}
		return true
	}
}

func IsNotNil(obj any) PredicateFunc {
	return func() bool {
		return obj != nil
	}
}

func IsNil(obj any) PredicateFunc {
	return func() bool {
		return obj == nil
	}
}

func IsEmptyString(str string) PredicateFunc {
	return func() bool {
		return str == ""
	}
}

func IsNotFunc(container func() bool) PredicateFunc {
	return func() bool {
		return !container()
	}
}

func BoolValue(value bool) PredicateFunc {
	//cautious using this, cannot reference cluster values as they are not yet initialized when creating structs
	//this is convenient for use with global configuration values
	return func() bool {
		return value
	}
}
