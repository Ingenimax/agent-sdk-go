# Social Media Content Team -- Multi-Agent A2A Example

This example demonstrates the A2A (Agent-to-Agent) protocol with multiple specialized agents running as independent servers, composed by an orchestrator that uses them as remote tools.

## Architecture

```
User provides topic via CLI arg
  |
  v
[Content Director]  orchestrator/  (local agent with 3 remote tools)
  |
  |-- A2A --> [Copywriter]    :9110  (drafts platform-specific posts)
  |-- A2A --> [Reviewer]      :9111  (reviews tone, accuracy, engagement)
  |-- A2A --> [Hashtag Guru]  :9112  (hashtags + platform optimization tips)
  |
  v
Final polished posts for Twitter/X, LinkedIn, Instagram
```

## Prerequisites

- Go 1.24+
- An OpenAI API key (set `OPENAI_API_KEY`)
- Optionally set `OPENAI_MODEL` (defaults to `gpt-4o-mini`)

## Running the Example

### 1. Start the specialist agents

Open three separate terminals:

```bash
# Terminal 1 -- Copywriter
OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/copywriter

# Terminal 2 -- Reviewer
OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/reviewer

# Terminal 3 -- Hashtag Guru
OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/hashtag
```

### 2. Verify agent discovery

```bash
curl -s http://localhost:9110/.well-known/agent-card.json | jq .name
# "Copywriter"

curl -s http://localhost:9111/.well-known/agent-card.json | jq .name
# "Reviewer"

curl -s http://localhost:9112/.well-known/agent-card.json | jq .name
# "HashtagGuru"
```

### 3. Run the orchestrator

```bash
OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/orchestrator "Launch of our new AI-powered code review tool"
```

The Content Director will:
1. Send the topic to the Copywriter to draft posts for Twitter/X, LinkedIn, and Instagram
2. Send the drafts to the Reviewer for editorial feedback and revision
3. Send the revised posts to the Hashtag Guru for hashtags and optimization tips
4. Present the final polished result

## Key Patterns Demonstrated

- **Real LLM agents** -- each agent uses `agent.NewAgent` with an OpenAI LLM
- **A2A agent discovery** -- the orchestrator discovers agents via their `/.well-known/agent-card.json`
- **Remote agents as tools** -- `NewRemoteAgentTool` wraps each A2A agent as an SDK tool
- **Agent cards with skills** -- each agent describes its capabilities in its card
- **Multi-process communication** -- four separate Go processes talking A2A over HTTP
- **Orchestration pipeline** -- the director coordinates specialists in a defined sequence
