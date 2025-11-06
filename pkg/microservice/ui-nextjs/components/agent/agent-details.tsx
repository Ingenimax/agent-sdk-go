'use client';

import React, { useState, useEffect } from 'react';
import { DetailedAgentConfig, Tool } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  ArrowLeft,
  Bot,
  Brain,
  Settings,
  Zap,
  Database,
  Network,
  Activity,
  Layers,
} from 'lucide-react';

interface AgentDetailsProps {
  onBack: () => void;
}

export function AgentDetails({ onBack }: AgentDetailsProps) {
  const [agentConfig, setAgentConfig] = useState<DetailedAgentConfig | null>(null);
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadAgentDetails();
  }, []);

  const loadAgentDetails = async () => {
    try {
      setLoading(true);

      // Load basic config and tools in parallel
      const [configResponse, toolsResponse] = await Promise.all([
        agentAPI.getAgentConfig(),
        agentAPI.getTools()
      ]);

      // For now, map basic config to detailed config
      // In the future, we'll extend the API to return more details
      const detailedConfig: DetailedAgentConfig = {
        ...configResponse,
        llm_config: {
          temperature: 0.7,
          top_p: 1.0,
          frequency_penalty: 0,
          presence_penalty: 0,
          enable_reasoning: false,
        },
        max_iterations: 2,
        require_plan_approval: false,
        is_remote: false,
        execution_plan_enabled: false,
        guardrails_enabled: false,
        tracing_enabled: false,
        created_at: new Date().toISOString(),
        version: "1.0.0",
      };

      setAgentConfig(detailedConfig);
      setTools(toolsResponse.tools);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agent details');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-gray-900"></div>
          <p className="mt-4 text-lg">Loading Agent Details...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-red-500 text-lg mb-4">Error: {error}</p>
          <Button onClick={loadAgentDetails}>Retry</Button>
        </div>
      </div>
    );
  }

  if (!agentConfig) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-muted-foreground">No agent configuration found</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="h-14 border-b border-border flex items-center px-4 bg-background/95 backdrop-blur">
        <Button variant="ghost" size="icon" onClick={onBack} className="mr-2">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-lg font-semibold">Agent Details</h1>
      </div>

      {/* Content */}
      <ScrollArea className="flex-1 p-6">
        <div className="max-w-6xl mx-auto space-y-6">
          {/* Agent Overview */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <Bot className="h-5 w-5" />
                <span>Agent Overview</span>
              </CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Name</label>
                  <p className="text-lg font-semibold">{agentConfig.name}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Description</label>
                  <p className="text-sm">{agentConfig.description}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Model</label>
                  <Badge variant="outline" className="text-sm">{agentConfig.model}</Badge>
                </div>
              </div>
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Version</label>
                  <p className="text-sm">{agentConfig.version}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Created</label>
                  <p className="text-sm">{new Date(agentConfig.created_at || '').toLocaleDateString()}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Status</label>
                  <div className="flex items-center space-x-2">
                    <div className="h-2 w-2 bg-green-500 rounded-full"></div>
                    <span className="text-sm">Active</span>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Detailed Configuration */}
          <Tabs defaultValue="llm" className="w-full">
            <TabsList className="grid w-full grid-cols-6">
              <TabsTrigger value="llm">LLM Config</TabsTrigger>
              <TabsTrigger value="tools">Tools</TabsTrigger>
              <TabsTrigger value="memory">Memory</TabsTrigger>
              <TabsTrigger value="execution">Execution</TabsTrigger>
              <TabsTrigger value="system">System</TabsTrigger>
              <TabsTrigger value="advanced">Advanced</TabsTrigger>
            </TabsList>

            {/* LLM Configuration */}
            <TabsContent value="llm" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Brain className="h-5 w-5" />
                    <span>LLM Configuration</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Temperature</label>
                      <p className="text-sm">{agentConfig.llm_config?.temperature || 'Not set'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Top P</label>
                      <p className="text-sm">{agentConfig.llm_config?.top_p || 'Not set'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Frequency Penalty</label>
                      <p className="text-sm">{agentConfig.llm_config?.frequency_penalty || 'Not set'}</p>
                    </div>
                  </div>
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Presence Penalty</label>
                      <p className="text-sm">{agentConfig.llm_config?.presence_penalty || 'Not set'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Reasoning Enabled</label>
                      <Badge variant={agentConfig.llm_config?.enable_reasoning ? 'default' : 'secondary'}>
                        {agentConfig.llm_config?.enable_reasoning ? 'Yes' : 'No'}
                      </Badge>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Reasoning Budget</label>
                      <p className="text-sm">{agentConfig.llm_config?.reasoning_budget || 'Not set'}</p>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>System Prompt</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="bg-muted p-4 rounded-md">
                    <pre className="text-sm whitespace-pre-wrap">{agentConfig.system_prompt}</pre>
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            {/* Tools */}
            <TabsContent value="tools" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Zap className="h-5 w-5" />
                    <span>Available Tools ({tools.length})</span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {tools.length > 0 ? (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {tools.map((tool, index) => (
                        <Card key={index} className="border">
                          <CardContent className="p-4">
                            <div className="flex items-center justify-between mb-2">
                              <h4 className="font-medium">{tool.name}</h4>
                              <Badge variant={tool.enabled ? 'default' : 'secondary'}>
                                {tool.enabled ? 'Enabled' : 'Disabled'}
                              </Badge>
                            </div>
                            <p className="text-sm text-muted-foreground">{tool.description}</p>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  ) : (
                    <p className="text-muted-foreground">No tools configured</p>
                  )}
                </CardContent>
              </Card>

              {agentConfig.sub_agents && agentConfig.sub_agents.length > 0 && (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center space-x-2">
                      <Layers className="h-5 w-5" />
                      <span>Sub-Agents ({agentConfig.sub_agents.length})</span>
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-3">
                      {agentConfig.sub_agents.map((subAgent) => (
                        <Card key={subAgent.id} className="border">
                          <CardContent className="p-4">
                            <div className="flex items-center justify-between mb-2">
                              <h4 className="font-medium">{subAgent.name}</h4>
                              <Badge variant={subAgent.status === 'active' ? 'default' : 'secondary'}>
                                {subAgent.status}
                              </Badge>
                            </div>
                            <p className="text-sm text-muted-foreground mb-2">{subAgent.description}</p>
                            <div className="flex items-center space-x-2">
                              <Badge variant="outline" className="text-xs">{subAgent.model}</Badge>
                              <Badge variant="outline" className="text-xs">{subAgent.tools.length} tools</Badge>
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              )}
            </TabsContent>

            {/* Memory */}
            <TabsContent value="memory" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Database className="h-5 w-5" />
                    <span>Memory Configuration</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Type</label>
                      <Badge variant="outline">{agentConfig.memory.type}</Badge>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Status</label>
                      <Badge variant={agentConfig.memory.status === 'active' ? 'default' : 'secondary'}>
                        {agentConfig.memory.status}
                      </Badge>
                    </div>
                  </div>
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Entry Count</label>
                      <p className="text-sm">{agentConfig.memory.entry_count || 'N/A'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Max Capacity</label>
                      <p className="text-sm">{agentConfig.memory.max_capacity || 'Unlimited'}</p>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            {/* Execution */}
            <TabsContent value="execution" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Activity className="h-5 w-5" />
                    <span>Execution Configuration</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Max Iterations</label>
                      <p className="text-sm">{agentConfig.max_iterations}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Plan Approval Required</label>
                      <Badge variant={agentConfig.require_plan_approval ? 'default' : 'secondary'}>
                        {agentConfig.require_plan_approval ? 'Yes' : 'No'}
                      </Badge>
                    </div>
                  </div>
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Execution Plans</label>
                      <Badge variant={agentConfig.execution_plan_enabled ? 'default' : 'secondary'}>
                        {agentConfig.execution_plan_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Guardrails</label>
                      <Badge variant={agentConfig.guardrails_enabled ? 'default' : 'secondary'}>
                        {agentConfig.guardrails_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            {/* System */}
            <TabsContent value="system" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Network className="h-5 w-5" />
                    <span>System Configuration</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Remote Agent</label>
                      <Badge variant={agentConfig.is_remote ? 'default' : 'secondary'}>
                        {agentConfig.is_remote ? 'Yes' : 'No'}
                      </Badge>
                    </div>
                    {agentConfig.is_remote && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">Remote URL</label>
                        <p className="text-sm font-mono">{agentConfig.remote_url}</p>
                      </div>
                    )}
                  </div>
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Tracing</label>
                      <Badge variant={agentConfig.tracing_enabled ? 'default' : 'secondary'}>
                        {agentConfig.tracing_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                    {agentConfig.remote_timeout && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">Remote Timeout</label>
                        <p className="text-sm">{agentConfig.remote_timeout}ms</p>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            {/* Advanced */}
            <TabsContent value="advanced" className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Settings className="h-5 w-5" />
                    <span>Advanced Configuration</span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    {agentConfig.response_format && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">Response Format</label>
                        <div className="mt-2 p-3 bg-muted rounded-md">
                          <p className="text-sm"><strong>Type:</strong> {agentConfig.response_format.type}</p>
                          {agentConfig.response_format.schema_name && (
                            <p className="text-sm"><strong>Schema:</strong> {agentConfig.response_format.schema_name}</p>
                          )}
                        </div>
                      </div>
                    )}

                    {agentConfig.metadata && Object.keys(agentConfig.metadata).length > 0 && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">Metadata</label>
                        <div className="mt-2 p-3 bg-muted rounded-md">
                          <pre className="text-sm">{JSON.stringify(agentConfig.metadata, null, 2)}</pre>
                        </div>
                      </div>
                    )}

                    {agentConfig.features && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">UI Features</label>
                        <div className="mt-2 flex flex-wrap gap-2">
                          <Badge variant={agentConfig.features.chat ? 'default' : 'secondary'}>
                            Chat: {agentConfig.features.chat ? 'Enabled' : 'Disabled'}
                          </Badge>
                          <Badge variant={agentConfig.features.memory ? 'default' : 'secondary'}>
                            Memory: {agentConfig.features.memory ? 'Enabled' : 'Disabled'}
                          </Badge>
                          <Badge variant={agentConfig.features.agent_info ? 'default' : 'secondary'}>
                            Agent Info: {agentConfig.features.agent_info ? 'Enabled' : 'Disabled'}
                          </Badge>
                          <Badge variant={agentConfig.features.settings ? 'default' : 'secondary'}>
                            Settings: {agentConfig.features.settings ? 'Enabled' : 'Disabled'}
                          </Badge>
                        </div>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </div>
      </ScrollArea>
    </div>
  );
}