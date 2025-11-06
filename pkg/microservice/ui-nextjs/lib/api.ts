import {
  AgentConfig,
  SubAgentInfo,
  Tool,
  MemoryEntry,
  MemoryResponse,
  ConversationInfo,
  RunRequest,
  StreamRequest,
  RunResponse,
  StreamResponse,
  StreamEventData
} from '@/types/agent';

const API_BASE_URL = '/api/v1';

class AgentAPI {
  private baseUrl: string;

  constructor(baseUrl = API_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  async get<T>(endpoint: string): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`);
    if (!response.ok) {
      throw new Error(`API Error: ${response.status} ${response.statusText}`);
    }
    return response.json();
  }

  async post<T>(endpoint: string, data: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(data),
    });
    if (!response.ok) {
      throw new Error(`API Error: ${response.status} ${response.statusText}`);
    }
    return response.json();
  }

  // Agent configuration endpoints
  async getAgentConfig(): Promise<AgentConfig> {
    return this.get<AgentConfig>('/agent/config');
  }

  async getAgentMetadata(): Promise<AgentConfig> {
    return this.get<AgentConfig>('/agent/metadata');
  }

  // Sub-agents endpoints
  async getSubAgents(): Promise<{ sub_agents: SubAgentInfo[]; count: number }> {
    return this.get<{ sub_agents: SubAgentInfo[]; count: number }>('/agent/subagents');
  }

  async getSubAgent(id: string): Promise<SubAgentInfo> {
    return this.get<SubAgentInfo>(`/agent/subagents/${id}`);
  }

  async delegateToSubAgent(data: {
    sub_agent_id: string;
    task: string;
    context?: Record<string, string>;
    conversation_id?: string;
  }): Promise<{
    status: string;
    sub_agent_id: string;
    task: string;
    result: string;
  }> {
    return this.post('/agent/delegate', data);
  }

  // Tools endpoints
  async getTools(): Promise<{ tools: Tool[]; count: number }> {
    return this.get<{ tools: Tool[]; count: number }>('/tools');
  }

  // Memory endpoints
  async getMemory(limit = 100, offset = 0): Promise<MemoryResponse> {
    return this.get(`/memory?limit=${limit}&offset=${offset}`);
  }

  async getConversationMessages(conversationId: string, limit = 100, offset = 0): Promise<MemoryResponse> {
    return this.get(`/memory?conversation_id=${conversationId}&limit=${limit}&offset=${offset}`);
  }

  async searchMemory(query: string): Promise<{
    query: string;
    results: MemoryEntry[];
    count: number;
  }> {
    return this.get(`/memory/search?q=${encodeURIComponent(query)}`);
  }

  // Chat endpoints
  async runAgent(data: RunRequest): Promise<RunResponse> {
    return this.post<RunResponse>('/agent/run', data);
  }

  // Streaming chat with SSE
  async *streamAgent(data: StreamRequest): AsyncGenerator<StreamEventData, void, unknown> {
    const response = await fetch(`${this.baseUrl}/agent/stream`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'text/event-stream',
      },
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      throw new Error(`Stream Error: ${response.status} ${response.statusText}`);
    }

    const reader = response.body?.getReader();
    const decoder = new TextDecoder();

    if (!reader) {
      throw new Error('No response body reader available');
    }

    try {
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');

        // Keep the last line in buffer as it might be incomplete
        buffer = lines.pop() || '';

        let currentEvent: Partial<StreamResponse> = {};

        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent.event = line.slice(7);
          } else if (line.startsWith('data: ')) {
            const dataStr = line.slice(6);
            if (dataStr === '[DONE]') {
              return;
            }
            try {
              currentEvent.data = JSON.parse(dataStr) as StreamEventData;
            } catch {
              console.warn('Failed to parse SSE data:', dataStr);
            }
          } else if (line.startsWith('id: ')) {
            currentEvent.id = line.slice(4);
          } else if (line === '') {
            // End of event, yield if we have data
            if (currentEvent.data) {
              yield currentEvent.data;
              currentEvent = {};
            }
          }
        }
      }
    } finally {
      reader.releaseLock();
    }
  }

  // Health check
  async health(): Promise<{ status: string; timestamp: number }> {
    const response = await fetch('/health');
    if (!response.ok) {
      throw new Error(`Health check failed: ${response.status}`);
    }
    return response.json();
  }
}

export const agentAPI = new AgentAPI();
export default agentAPI;