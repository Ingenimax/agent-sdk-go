package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The core types carry no logic, so these tests pin their JSON wire format.

func TestTaskJSONOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(Task{ID: "t1", Status: StatusPending})
	require.NoError(t, err)

	var fields map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &fields))
	for _, key := range []string{"id", "name", "description", "status", "steps", "user_id", "created_at", "updated_at"} {
		assert.Contains(t, fields, key)
	}
	for _, key := range []string{"plan", "completed_at", "failed_at", "conversation_id", "input", "output", "metadata"} {
		assert.NotContains(t, fields, key)
	}
	assert.Equal(t, "pending", fields["status"])
}

func TestTaskJSONRoundTrip(t *testing.T) {
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	in := Task{
		ID:             "t1",
		Name:           "n",
		Description:    "d",
		Status:         StatusCompleted,
		UserID:         "u",
		Plan:           "p",
		CreatedAt:      now,
		UpdatedAt:      now,
		CompletedAt:    &now,
		ConversationID: "c",
		Input:          map[string]interface{}{"i": "x"},
		Output:         map[string]interface{}{"o": "y"},
		Metadata:       map[string]interface{}{"m": "z"},
		Steps: []*Step{{
			ID: "s1", Name: "sn", Type: "chat", Status: StatusFailed, Error: "bad",
			FailedAt: &now, OrderIndex: 2, Context: map[string]interface{}{"k": "v"},
		}},
	}

	data, err := json.Marshal(in)
	require.NoError(t, err)
	var out Task
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, in, out)
}

func TestRequestJSONFieldNames(t *testing.T) {
	var req CreateTaskRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"n","user_id":"u","conversation_id":"c"}`), &req))
	assert.Equal(t, CreateTaskRequest{Name: "n", UserID: "u", ConversationID: "c"}, req)

	var approve ApproveTaskPlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"approved":true,"feedback":"f"}`), &approve))
	assert.Equal(t, ApproveTaskPlanRequest{Approved: true, Feedback: "f"}, approve)

	data, err := json.Marshal(TaskFilter{UserID: "u", Limit: 5})
	require.NoError(t, err)
	assert.JSONEq(t, `{"user_id":"u","limit":5}`, string(data))
}
