'use client';

import React, { useState, useEffect } from 'react';
import { AgentConfig } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Settings, Moon, Sun, Zap, RefreshCw, CheckCircle, AlertCircle } from 'lucide-react';

interface SettingsScreenProps {
  agentConfig: AgentConfig | null;
}

export function SettingsScreen({ agentConfig }: SettingsScreenProps) {
  const [theme, setTheme] = useState<'light' | 'dark'>('light');
  const [streamingEnabled, setStreamingEnabled] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [healthStatus, setHealthStatus] = useState<'checking' | 'healthy' | 'error'>('checking');
  const [lastHealthCheck, setLastHealthCheck] = useState<Date | null>(null);

  useEffect(() => {
    checkHealth();
    const interval = setInterval(checkHealth, 30000); // Check every 30 seconds
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    // Apply theme
    if (theme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [theme]);

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

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="p-6 border-b border-border">
        <div className="flex items-center gap-2">
          <Settings className="h-6 w-6" />
          <h1 className="text-2xl font-bold">Settings</h1>
        </div>
      </div>

      <ScrollArea className="flex-1">
        <div className="p-6 space-y-6">
          {/* Appearance */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {theme === 'dark' ? <Moon className="h-5 w-5" /> : <Sun className="h-5 w-5" />}
                Appearance
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <label className="text-sm font-medium">Theme</label>
                  <p className="text-xs text-muted-foreground">Choose your preferred theme</p>
                </div>
                <div className="flex items-center gap-2">
                  <Sun className="h-4 w-4" />
                  <Switch
                    checked={theme === 'dark'}
                    onCheckedChange={(checked) => setTheme(checked ? 'dark' : 'light')}
                  />
                  <Moon className="h-4 w-4" />
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Chat Settings */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="h-5 w-5" />
                Chat Settings
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <label className="text-sm font-medium">Streaming Responses</label>
                  <p className="text-xs text-muted-foreground">
                    Enable real-time streaming of agent responses
                  </p>
                </div>
                <Switch
                  checked={streamingEnabled}
                  onCheckedChange={setStreamingEnabled}
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <label className="text-sm font-medium">Auto-refresh Data</label>
                  <p className="text-xs text-muted-foreground">
                    Automatically refresh agent data periodically
                  </p>
                </div>
                <Switch
                  checked={autoRefresh}
                  onCheckedChange={setAutoRefresh}
                />
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

          {/* Agent Configuration Summary */}
          {agentConfig && (
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
          )}

          {/* Environment Information */}
          <Card>
            <CardHeader>
              <CardTitle>Environment</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="text-sm space-y-1">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">UI Version:</span>
                  <span className="font-mono">1.0.0</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Framework:</span>
                  <span className="font-mono">Next.js 16 + React 19</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Components:</span>
                  <span className="font-mono">shadcn/ui + Tailwind CSS</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Last Updated:</span>
                  <span className="font-mono">{new Date().toLocaleDateString()}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </ScrollArea>
    </div>
  );
}