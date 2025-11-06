'use client';

import React, { useState, useEffect } from 'react';
import { AgentConfig } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';
import { Bot, Brain, Database, Settings, RefreshCw, CheckCircle, AlertCircle } from 'lucide-react';

interface AgentInfoScreenProps {
  agentConfig: AgentConfig | null;
}

export function AgentInfoScreen({ agentConfig }: AgentInfoScreenProps) {
  const [healthStatus, setHealthStatus] = useState<'checking' | 'healthy' | 'error'>('checking');
  const [lastHealthCheck, setLastHealthCheck] = useState<Date | null>(null);

  useEffect(() => {
    checkHealth();
    const interval = setInterval(checkHealth, 30000);
    return () => clearInterval(interval);
  }, []);

  const checkHealth = async () => {
    try {
      setHealthStatus('checking');
      await agentAPI.health();
      setHealthStatus('healthy');
      setLastHealthCheck(new Date());
    } catch (error) {
      setHealthStatus('error');
      setLastHealthCheck(new Date());
    }
  };

  const getHealthStatusIcon = () => {
    switch (healthStatus) {
      case 'checking':
        return <RefreshCw className="h-4 w-4 animate-spin" />;
      case 'healthy':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'error':
        return <AlertCircle className="h-4 w-4 text-red-500" />;
    }
  };

  const getHealthStatusText = () => {
    switch (healthStatus) {
      case 'checking':
        return 'Checking...';
      case 'healthy':
        return 'Healthy';
      case 'error':
        return 'Error';
    }
  };

  if (!agentConfig) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <Bot className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
          <p className="text-lg text-muted-foreground">No agent configuration available</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-hidden">
      <ScrollArea className="h-full">
        <div className="p-6 space-y-6 pb-8">
        {/* Agent Overview */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Bot className="h-5 w-5" />
              Agent Overview
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium text-muted-foreground">Name</label>
              <p className="text-lg font-semibold">{agentConfig.name}</p>
            </div>
            {agentConfig.description && (
              <div>
                <label className="text-sm font-medium text-muted-foreground">Description</label>
                <p className="text-sm">{agentConfig.description}</p>
              </div>
            )}
            <div>
              <label className="text-sm font-medium text-muted-foreground">Model</label>
              <div className="flex items-center gap-2 mt-1">
                <Badge variant="secondary">{agentConfig.model}</Badge>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* System Prompt */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Brain className="h-5 w-5" />
              System Prompt
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="bg-muted/50 p-4 rounded-lg">
              <pre className="whitespace-pre-wrap text-sm">
                {agentConfig.system_prompt || 'No system prompt configured'}
              </pre>
            </div>
          </CardContent>
        </Card>

        {/* Memory Information */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Memory System
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-sm font-medium text-muted-foreground">Type</label>
                <p className="text-sm font-semibold capitalize">{agentConfig.memory.type}</p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">Status</label>
                <Badge variant={agentConfig.memory.status === 'active' ? 'default' : 'secondary'}>
                  {agentConfig.memory.status}
                </Badge>
              </div>
              {agentConfig.memory.entry_count !== undefined && (
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Entry Count</label>
                  <p className="text-sm font-semibold">{agentConfig.memory.entry_count}</p>
                </div>
              )}
              {agentConfig.memory.max_capacity !== undefined && (
                <div>
                  <label className="text-sm font-medium text-muted-foreground">Max Capacity</label>
                  <p className="text-sm font-semibold">{agentConfig.memory.max_capacity}</p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>

        {/* System Health */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Settings className="h-5 w-5" />
                System Health
              </div>
              <Button variant="outline" size="sm" onClick={checkHealth} disabled={healthStatus === 'checking'}>
                <RefreshCw className={`h-4 w-4 mr-2 ${healthStatus === 'checking' ? 'animate-spin' : ''}`} />
                Check
              </Button>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <label className="text-sm font-medium">API Status</label>
                <p className="text-xs text-muted-foreground">
                  {lastHealthCheck ? `Last checked: ${lastHealthCheck.toLocaleTimeString()}` : 'Not checked yet'}
                </p>
              </div>
              <div className="flex items-center gap-2">
                {getHealthStatusIcon()}
                <Badge variant={healthStatus === 'healthy' ? 'default' : healthStatus === 'error' ? 'destructive' : 'secondary'}>
                  {getHealthStatusText()}
                </Badge>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Configuration Summary */}
        <Card>
          <CardHeader>
            <CardTitle>Configuration Summary</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-sm font-medium text-muted-foreground">Agent Name</label>
                <p className="text-sm font-semibold">{agentConfig.name}</p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">Model</label>
                <p className="text-sm font-semibold">{agentConfig.model}</p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">Tools Count</label>
                <p className="text-sm font-semibold">{agentConfig.tools.length}</p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">Memory Type</label>
                <p className="text-sm font-semibold capitalize">{agentConfig.memory.type}</p>
              </div>
            </div>

            <div>
              <label className="text-sm font-medium text-muted-foreground mb-2 block">
                Enabled Features
              </label>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                <Badge variant={agentConfig.features.chat ? 'default' : 'secondary'}>
                  Chat
                </Badge>
                <Badge variant={agentConfig.features.memory ? 'default' : 'secondary'}>
                  Memory
                </Badge>
                <Badge variant={agentConfig.features.agent_info ? 'default' : 'secondary'}>
                  Info
                </Badge>
                <Badge variant={agentConfig.features.settings ? 'default' : 'secondary'}>
                  Settings
                </Badge>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Features */}
        {/* <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings className="h-5 w-5" />
              Available Features
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
              <Badge variant={agentConfig.features.chat ? 'default' : 'secondary'}>
                Chat: {agentConfig.features.chat ? 'Enabled' : 'Disabled'}
              </Badge>
              <Badge variant={agentConfig.features.memory ? 'default' : 'secondary'}>
                Memory: {agentConfig.features.memory ? 'Enabled' : 'Disabled'}
              </Badge>
              <Badge variant={agentConfig.features.agent_info ? 'default' : 'secondary'}>
                Info: {agentConfig.features.agent_info ? 'Enabled' : 'Disabled'}
              </Badge>
              <Badge variant={agentConfig.features.settings ? 'default' : 'secondary'}>
                Settings: {agentConfig.features.settings ? 'Enabled' : 'Disabled'}
              </Badge>
            </div>
          </CardContent>
        </Card> */}

        {/* Sub-Agents */}
        {agentConfig.sub_agents && agentConfig.sub_agents.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Sub-Agents</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {agentConfig.sub_agents.map((subAgent) => (
                  <Card key={subAgent.id} className="border-dashed">
                    <CardContent className="p-4">
                      <div className="flex justify-between items-start">
                        <div>
                          <h4 className="font-semibold">{subAgent.name}</h4>
                          <p className="text-sm text-muted-foreground">{subAgent.description}</p>
                        </div>
                        <Badge variant="outline">{subAgent.status}</Badge>
                      </div>
                      <div className="flex gap-2 mt-2">
                        <Badge variant="secondary">{subAgent.model}</Badge>
                        <Badge variant="outline">{subAgent.tools.length} tools</Badge>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
        </div>
      </ScrollArea>
    </div>
  );
}