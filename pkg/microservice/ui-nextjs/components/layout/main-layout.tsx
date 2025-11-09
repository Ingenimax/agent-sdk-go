'use client';

import React, { useState, useEffect } from 'react';
import { ChatArea } from '../chat/chat-area';
import { AgentInfoScreen } from '../screens/agent-info-screen';
import { ToolsScreen } from '../screens/tools-screen';
import { MemoryScreen } from '../screens/memory-screen';
import { SubAgentsScreen } from '../screens/sub-agents-screen';
import { SettingsScreen } from '../screens/settings-screen';
import { TracesScreen } from '../screens/traces-screen';
import { AgentConfig } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { MessageSquare, Bot, Wrench, Database, Users, Settings, Activity } from 'lucide-react';

type ActiveScreen = 'chat' | 'agent-info' | 'tools' | 'memory' | 'sub-agents' | 'traces' | 'settings';

interface NavigationItem {
  id: ActiveScreen;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  description: string;
}

const navigationItems: NavigationItem[] = [
  {
    id: 'chat',
    label: 'Chat',
    icon: MessageSquare,
    description: 'Chat with the agent'
  },
  {
    id: 'agent-info',
    label: 'Agent Info',
    icon: Bot,
    description: 'View agent configuration and details'
  },
  {
    id: 'tools',
    label: 'Tools',
    icon: Wrench,
    description: 'Available tools and capabilities'
  },
  {
    id: 'memory',
    label: 'Memory',
    icon: Database,
    description: 'Browse conversation history and traces'
  },
  {
    id: 'sub-agents',
    label: 'Sub-Agents',
    icon: Users,
    description: 'Manage and delegate tasks to sub-agents'
  },
  {
    id: 'traces',
    label: 'Traces',
    icon: Activity,
    description: 'Monitor agent execution traces and performance'
  },
  {
    id: 'settings',
    label: 'Settings',
    icon: Settings,
    description: 'Application settings and preferences'
  }
];

export function MainLayout() {
  const [activeScreen, setActiveScreen] = useState<ActiveScreen>('chat');
  const [agentConfig, setAgentConfig] = useState<AgentConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadAgentConfig();
  }, []);

  const loadAgentConfig = async () => {
    try {
      setLoading(true);
      const config = await agentAPI.getAgentConfig();
      setAgentConfig(config);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agent config');
    } finally {
      setLoading(false);
    }
  };

  const renderActiveScreen = () => {
    switch (activeScreen) {
      case 'chat':
        return <ChatArea agentConfig={agentConfig} />;
      case 'agent-info':
        return <AgentInfoScreen agentConfig={agentConfig} />;
      case 'tools':
        return <ToolsScreen />;
      case 'memory':
        return <MemoryScreen />;
      case 'sub-agents':
        return <SubAgentsScreen />;
      case 'traces':
        return <TracesScreen />;
      case 'settings':
        return <SettingsScreen agentConfig={agentConfig} />;
      default:
        return <ChatArea agentConfig={agentConfig} />;
    }
  };

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-gray-900"></div>
          <p className="mt-4 text-lg">Loading Agent UI...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center">
          <p className="text-red-500 text-lg mb-4">Error: {error}</p>
          <Button onClick={loadAgentConfig}>Retry</Button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full bg-background flex flex-col fixed inset-0">
      {/* Header */}
      <header className="h-16 border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 flex-shrink-0">
        <div className="flex items-center justify-between h-full px-4">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <Bot className="h-6 w-6" />
              <h1 className="text-xl font-semibold">
                {agentConfig?.name || 'Agent'}
              </h1>
            </div>
            {agentConfig && (
              <Badge variant="secondary" className="text-xs">
                {agentConfig.model}
              </Badge>
            )}
          </div>

          <div className="flex items-center space-x-2">
            <div className="flex items-center space-x-1">
              <div className="h-2 w-2 bg-green-500 rounded-full"></div>
              <span className="text-sm text-muted-foreground">Ready</span>
            </div>
          </div>
        </div>
      </header>

      {/* Navigation */}
      <nav className="border-b border-border bg-background px-4">
        <div className="flex space-x-1 py-2">
          {navigationItems
            .filter((item) => {
              // Only show traces tab if the feature is enabled
              if (item.id === 'traces') {
                return agentConfig?.features?.traces === true;
              }
              return true;
            })
            .map((item) => {
            const Icon = item.icon;
            const isActive = activeScreen === item.id;

            return (
              <Button
                key={item.id}
                variant={isActive ? 'default' : 'ghost'}
                size="sm"
                onClick={() => setActiveScreen(item.id)}
                className={`flex items-center gap-2 ${isActive ? '' : 'text-muted-foreground'}`}
                title={item.description}
              >
                <Icon className="h-4 w-4" />
                <span className="hidden sm:inline">{item.label}</span>
              </Button>
            );
          })}
        </div>
      </nav>

      {/* Main Content */}
      <main className="flex-1 overflow-auto">
        <div className="h-full overflow-y-auto">
          {renderActiveScreen()}
        </div>
      </main>
    </div>
  );
}