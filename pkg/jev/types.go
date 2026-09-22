// Package jev provides a client for TypeSafe AI's Jev System One API.
//
// Jev is a decision model, not an LLM: it evaluates state against named,
// typed questions and returns probabilities rather than generated text.
package jev

import "fmt"

// QuestionType identifies the shape of a Jev question and its answer.
type QuestionType string

const (
	// QuestionTypeNoul is a yes/no judgment returned as the probability of yes.
	QuestionTypeNoul QuestionType = "noul"
	// QuestionTypeChoice selects one label from caller-provided criteria.
	QuestionTypeChoice QuestionType = "choice"
	// QuestionTypeScore evaluates state against an ordered rubric.
	QuestionTypeScore QuestionType = "score"
)

// Question is implemented by the three Jev question primitives.
type Question interface {
	questionType() QuestionType
	validate() error
}

// NoulCriteria optionally describes the true and false outcomes.
type NoulCriteria struct {
	True  interface{} `json:"true,omitempty"`
	False interface{} `json:"false,omitempty"`
}

// NoulQuestion asks for the probability that a yes/no judgment is true.
type NoulQuestion struct {
	Type         QuestionType  `json:"type"`
	Instructions interface{}   `json:"instructions,omitempty"`
	Criteria     *NoulCriteria `json:"criteria,omitempty"`
}

func (NoulQuestion) questionType() QuestionType { return QuestionTypeNoul }
func (q NoulQuestion) validate() error {
	if q.Type != QuestionTypeNoul {
		return fmt.Errorf("noul question has type %q", q.Type)
	}
	return nil
}

// Noul creates a yes/no question.
func Noul(instructions interface{}) Question {
	return NoulQuestion{Type: QuestionTypeNoul, Instructions: instructions}
}

// NoulWithCriteria creates a yes/no question with descriptions of both outcomes.
func NoulWithCriteria(instructions, whenTrue, whenFalse interface{}) Question {
	return NoulQuestion{
		Type:         QuestionTypeNoul,
		Instructions: instructions,
		Criteria:     &NoulCriteria{True: whenTrue, False: whenFalse},
	}
}

// ChoiceQuestion asks Jev to select one of the named criteria.
type ChoiceQuestion struct {
	Type         QuestionType           `json:"type"`
	Instructions interface{}            `json:"instructions,omitempty"`
	Criteria     map[string]interface{} `json:"criteria"`
}

func (ChoiceQuestion) questionType() QuestionType { return QuestionTypeChoice }
func (q ChoiceQuestion) validate() error {
	if q.Type != QuestionTypeChoice {
		return fmt.Errorf("choice question has type %q", q.Type)
	}
	if len(q.Criteria) < 2 {
		return fmt.Errorf("choice question requires at least two criteria")
	}
	for label := range q.Criteria {
		if label == "" {
			return fmt.Errorf("choice criterion labels must not be empty")
		}
	}
	return nil
}

// Choice creates a question whose answer must be one of the criteria labels.
func Choice(instructions interface{}, criteria map[string]interface{}) Question {
	return ChoiceQuestion{
		Type:         QuestionTypeChoice,
		Instructions: instructions,
		Criteria:     criteria,
	}
}

// ScoreQuestion asks Jev to evaluate state against an ordered rubric.
type ScoreQuestion struct {
	Type         QuestionType  `json:"type"`
	Instructions interface{}   `json:"instructions,omitempty"`
	Criteria     []interface{} `json:"criteria"`
}

func (ScoreQuestion) questionType() QuestionType { return QuestionTypeScore }
func (q ScoreQuestion) validate() error {
	if q.Type != QuestionTypeScore {
		return fmt.Errorf("score question has type %q", q.Type)
	}
	if len(q.Criteria) < 2 {
		return fmt.Errorf("score question requires at least two rubric levels")
	}
	return nil
}

// Score creates an ordered rubric question. The first criterion is score zero.
func Score(instructions interface{}, criteria ...interface{}) Question {
	return ScoreQuestion{
		Type:         QuestionTypeScore,
		Instructions: instructions,
		Criteria:     criteria,
	}
}

// Request evaluates State against one or more named Questions.
type Request struct {
	State     interface{}         `json:"state"`
	Questions map[string]Question `json:"questions"`
	Model     string              `json:"model,omitempty"`
}

// Answer is one typed answer returned by Jev. Only fields matching Type are set.
type Answer struct {
	Type          QuestionType           `json:"type"`
	Noul          *float64               `json:"noul,omitempty"`
	Choice        string                 `json:"choice,omitempty"`
	Score         *float64               `json:"score,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	Probabilities map[string]float64     `json:"probabilities,omitempty"`
	Legend        map[string]interface{} `json:"legend,omitempty"`
}

// Usage reports request token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response contains answers keyed by request question name.
type Response struct {
	Model     string            `json:"model"`
	Answers   map[string]Answer `json:"answers"`
	Usage     Usage             `json:"usage"`
	RequestID string            `json:"-"`
}
