package consolidation

import (
	"context"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// scopeKey rebuilds the "orgID:conversationID" key memory scopes by.
func scopeKey(ctx context.Context) (string, bool) {
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil || orgID == "" {
		orgID = "default"
	}
	conversationID, ok := memory.GetConversationID(ctx)
	if !ok || conversationID == "" {
		return "", false
	}
	return orgID + ":" + conversationID, true
}

func splitScopeKey(key string) (orgID, conversationID string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "default", key
	}
	return parts[0], parts[1]
}

// withScope stamps the identity a memory read requires.
//
// This is why consolidation could not have been built before sessions: a
// background pass has no inbound request to inherit org and conversation
// identity from, and memory hard-fails without both.
func withScope(ctx context.Context, orgID, conversationID string) context.Context {
	if orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, orgID)
	}
	return memory.WithConversationID(ctx, conversationID)
}
