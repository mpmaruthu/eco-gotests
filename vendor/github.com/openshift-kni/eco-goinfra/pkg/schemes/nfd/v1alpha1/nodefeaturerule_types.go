/*
Copyright 2021. The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeFeatureRuleSpec defines the desired state of NodeFeatureRule
type NodeFeatureRuleSpec struct {
	// Rules is a list of node customization rules.
	Rules []Rule `json:"rules"`
}

// NodeFeatureRuleStatus defines the observed state of NodeFeatureRule
type NodeFeatureRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NodeFeatureRule resource specifies a configuration for feature-based
// customization of node objects, such as node labeling.
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=nfr,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:resource:scope=Namespaced
type NodeFeatureRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeFeatureRuleSpec   `json:"spec,omitempty"`
	Status NodeFeatureRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeFeatureRuleList contains a list of NodeFeatureRule objects.
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeFeatureRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeFeatureRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeFeatureRule{}, &NodeFeatureRuleList{})
}

// Rule defines a rule for node customization such as labeling.
type Rule struct {
	// Name of the rule.
	Name string `json:"name"`

	// Labels to create if the rule matches.
	// +optional
	Labels map[string]string `json:"labels"`

	// LabelsTemplate specifies a template to expand for dynamically generating
	// multiple labels. Data (after template expansion) must be keys with an
	// optional value (<key>[=<value>]) separated by newlines.
	// +optional
	LabelsTemplate string `json:"labelsTemplate"`

	// Annotations to create if the rule matches.
	// +optional
	Annotations map[string]string `json:"annotations"`

	// Vars is the variables to store if the rule matches. Variables do not
	// directly inflict any changes in the node object. However, they can be
	// referenced from other rules enabling more complex rule hierarchies,
	// without exposing intermediary output values as labels.
	// +optional
	Vars map[string]string `json:"vars"`

	// VarsTemplate specifies a template to expand for dynamically generating
	// multiple variables. Data (after template expansion) must be keys with an
	// optional value (<key>[=<value>]) separated by newlines.
	// +optional
	VarsTemplate string `json:"varsTemplate"`

	// Taints to create if the rule matches.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// ExtendedResources to create if the rule matches.
	// +optional
	ExtendedResources map[string]string `json:"extendedResources"`

	// MatchFeatures specifies a set of matcher terms all of which must match.
	// +optional
	MatchFeatures FeatureMatcher `json:"matchFeatures"`

	// MatchAny specifies a list of matchers one of which must match.
	// +optional
	MatchAny []MatchAnyElem `json:"matchAny"`
}

// MatchAnyElem specifies one sub-matcher of MatchAny.
type MatchAnyElem struct {
	// MatchFeatures specifies a set of matcher terms all of which must match.
	MatchFeatures FeatureMatcher `json:"matchFeatures"`
}

// FeatureMatcher specifies a set of feature matcher terms (i.e. per-feature
// matchers), all of which must match.
type FeatureMatcher []FeatureMatcherTerm

// FeatureMatcherTerm defines requirements against one feature set. All
// requirements (specified as MatchExpressions) are evaluated against each
// element in the feature set.
type FeatureMatcherTerm struct {
	// Feature is the name of the feature set to match against.
	Feature string `json:"feature"`
	// MatchExpressions is the set of per-element expressions evaluated. These
	// match against the value of the specified elements.
	// +optional
	MatchExpressions *MatchExpressionSet `json:"matchExpressions"`
	// MatchName in an expression that is matched against the name of each
	// element in the feature set.
	// +optional
	MatchName *MatchExpression `json:"matchName"`
}

// MatchExpressionSet contains a set of MatchExpressions, each of which is
// evaluated against a set of input values.
type MatchExpressionSet map[string]*MatchExpression

// MatchExpression specifies an expression to evaluate against a set of input
// values. It contains an operator that is applied when matching the input and
// an array of values that the operator evaluates the input against.
type MatchExpression struct {
	// Op is the operator to be applied.
	Op MatchOp `json:"op"`

	// Value is the list of values that the operand evaluates the input
	// against. Value should be empty if the operator is Exists, DoesNotExist,
	// IsTrue or IsFalse. Value should contain exactly one element if the
	// operator is Gt or Lt and exactly two elements if the operator is GtLt.
	// In other cases Value should contain at least one element.
	// +optional
	Value MatchValue `json:"value,omitempty"`
}

// MatchOp is the match operator that is applied on values when evaluating a
// MatchExpression.
// +kubebuilder:validation:Enum="In";"NotIn";"InRegexp";"Exists";"DoesNotExist";"Gt";"Lt";"GtLt";"IsTrue";"IsFalse"
type MatchOp string

// MatchValue is the list of values associated with a MatchExpression.
type MatchValue []string

const (
	// MatchAny returns always true.
	MatchAny MatchOp = ""
	// MatchIn returns true if any of the values stored in the expression is
	// equal to the input.
	MatchIn MatchOp = "In"
	// MatchNotIn returns true if none of the values in the expression are
	// equal to the input.
	MatchNotIn MatchOp = "NotIn"
	// MatchInRegexp treats values of the expression as regular expressions and
	// returns true if any of them matches the input.
	MatchInRegexp MatchOp = "InRegexp"
	// MatchExists returns true if the input is valid. The expression must not
	// have any values.
	MatchExists MatchOp = "Exists"
	// MatchDoesNotExist returns true if the input is not valid. The expression
	// must not have any values.
	MatchDoesNotExist MatchOp = "DoesNotExist"
	// MatchGt returns true if the input is greater than the value of the
	// expression (number of values in the expression must be exactly one).
	// Both the input and value must be integer numbers, otherwise an error is
	// returned.
	MatchGt MatchOp = "Gt"
	// MatchLt returns true if the input is less  than the value of the
	// expression (number of values in the expression must be exactly one).
	// Both the input and value must be integer numbers, otherwise an error is
	// returned.
	MatchLt MatchOp = "Lt"
	// MatchGtLt returns true if the input is between two values, i.e. greater
	// than the first value and less than the second value of the expression
	// (number of values in the expression must be exactly two). Both the input
	// and values must be integer numbers, otherwise an error is returned.
	MatchGtLt MatchOp = "GtLt"
	// MatchIsTrue returns true if the input holds the value "true". The
	// expression must not have any values.
	MatchIsTrue MatchOp = "IsTrue"
	// MatchIsFalse returns true if the input holds the value "false". The
	// expression must not have any values.
	MatchIsFalse MatchOp = "IsFalse"
)

const (
	// RuleBackrefDomain is the special feature domain for backreferencing
	// output of preceding rules.
	RuleBackrefDomain = "rule"
	// RuleBackrefFeature is the special feature name for backreferencing
	// output of preceding rules.
	RuleBackrefFeature = "matched"
)

// MatchAllNames is a special key in MatchExpressionSet to use field names
// (keys from the input) instead of values when matching.
const MatchAllNames = "*"
