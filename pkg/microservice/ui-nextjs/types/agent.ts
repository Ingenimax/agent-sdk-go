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