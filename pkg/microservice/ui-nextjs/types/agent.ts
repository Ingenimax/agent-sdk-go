export interface AgentInfo {
  name: string;
  description: string;
  model: string;
  system_prompt: string;
}

export interface SubAgentInfo {
  id: string;
  name: string;
  description: string;
  model: string;
  status: string;
  tools: string[];
  capabilities?: string[];
}

export interface MemoryInfo {
  type: string;
  status: string;
  entry_count?: number;
  max_capacity?: number;
}

export interface UIFeatures {
  chat: boolean;
  memory: boolean;
  agent_info: boolean;
  settings: boolean;
  traces: boolean;
}

export interface LLMConfig {
  temperature?: number;
  top_p?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
  stop_sequences?: string[];
  reasoning?: string;
  enable_reasoning?: boolean;
  reasoning_budget?: number;
}

export interface ResponseFormat {
  type: string;
  schema_name?: string;
  schema_definition?: Record<string, unknown>;
}

export interface StreamConfig {
  buffer_size?: number;
  flush_interval?: number;
  max_tokens?: number;
}

export interface MCPServerInfo {
  name: string;
  type: string;
  status: string;
  tools: string[];
}

export interface AgentConfig {
  name: string;
  description: string;
  model: string;
  system_prompt: string;
  tools: string[];
  memory: MemoryInfo;
  sub_agents?: SubAgentInfo[];
  features: UIFeatures;
  ui_theme?: string;
  metadata?: Record<string, unknown>;
}

export interface DetailedAgentConfig {
  // Basic Info
  name: string;
  description: string;
  model: string;
  system_prompt: string;

  // LLM Configuration
  llm_config?: LLMConfig;
  response_format?: ResponseFormat;
  stream_config?: StreamConfig;

  // Agent Configuration
  max_iterations?: number;
  require_plan_approval?: boolean;
  is_remote?: boolean;
  remote_url?: string;
  remote_timeout?: number;

  // Tools & Sub-agents
  tools: string[];
  sub_agents?: SubAgentInfo[];
  mcp_servers?: MCPServerInfo[];

  // Memory & Features
  memory: MemoryInfo;
  features: UIFeatures;

  // Execution
  execution_plan_enabled?: boolean;
  guardrails_enabled?: boolean;
  tracing_enabled?: boolean;

  // Metadata
  created_at?: string;
  updated_at?: string;
  version?: string;
  metadata?: Record<string, unknown>;
}

export interface Tool {
  name: string;
  description: string;
  enabled: boolean;
}

export interface MemoryEntry {
  id: string;
  role: string;
  content: string;
  timestamp: number;
  conversation_id?: string;
  metadata?: Record<string, unknown>;
}

export interface ConversationInfo {
  id: string;
  message_count: number;
  last_activity: number;
  last_message?: string;
}

export interface MemoryResponse {
  mode: 'conversations' | 'messages';
  conversations?: ConversationInfo[];
  messages?: MemoryEntry[];
  total: number;
  limit: number;
  offset: number;
  conversation_id?: string;
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: number;
  id?: string;
}

export interface StreamEventData {
  type: string;
  content?: string;
  thinking_step?: string;
  tool_call?: {
    id?: string;
    name: string;
    arguments?: string;
    result?: string;
    status: string;
  };
  error?: string;
  metadata?: Record<string, unknown>;
  is_final: boolean;
  timestamp: number;
}

export interface StreamResponse {
  event: string;
  data: StreamEventData;
  id?: string;
}

export interface RunRequest {
  input: string;
  conversation_id?: string;
  org_id?: string;
  context?: Record<string, string>;
  max_iterations?: number;
}

export interface StreamRequest {
  input: string;
  conversation_id?: string;
  org_id?: string;
  context?: Record<string, string>;
  max_iterations?: number;
}

export interface RunResponse {
  output: string;
  agent: string;
  metadata?: Record<string, unknown>;
}

// Trace-related types
export interface UITrace {
  id: string;
  name: string;
  start_time: string;
  end_time?: string;
  duration_ms: number;
  status: 'running' | 'completed' | 'error';
  spans: UITraceSpan[];
  metadata?: Record<string, unknown>;
  conversation_id?: string;
  org_id?: string;
  size_bytes: number;
}

export interface UITraceSpan {
  id: string;
  trace_id: string;
  parent_id?: string;
  name: string;
  type: 'generation' | 'tool_call' | 'span' | 'event';
  start_time: string;
  end_time?: string;
  duration_ms: number;
  events?: UITraceEvent[];
  attributes?: Record<string, unknown>;
  error?: UITraceError;
  input?: string;
  output?: string;
}

export interface UITraceEvent {
  name: string;
  timestamp: string;
  attributes?: Record<string, unknown>;
}

export interface UITraceError {
  message: string;
  type?: string;
  stacktrace?: string;
  timestamp: string;
}

export interface TracesResponse {
  traces: UITrace[];
  total: number;
  limit: number;
  offset: number;
}

export interface TraceStats {
  total_traces: number;
  running_traces: number;
  error_count: number;
  error_rate: number;
  avg_duration_ms: number;
  buffer_size_bytes: number;
  buffer_usage: number;
  tool_usage: Record<string, number>;
}