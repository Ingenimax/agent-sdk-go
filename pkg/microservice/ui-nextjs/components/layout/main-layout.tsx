'use client';

import React, { useState, useEffect } from 'react';
import { Sidebar } from './sidebar';
import { ChatArea } from '../chat/chat-area';
import { AgentConfig } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Menu } from 'lucide-react';

export function MainLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(true);
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
    <div className="flex h-full bg-background fixed inset-0">
      {/* Sidebar */}
      <div className={`
        ${sidebarOpen ? 'w-80' : 'w-0'}
        transition-all duration-300 ease-in-out
        overflow-hidden
        border-r border-border
        flex-shrink-0
      `}>
        <Sidebar
          agentConfig={agentConfig}
          isOpen={sidebarOpen}
          onClose={() => setSidebarOpen(false)}
        />
      </div>

      {/* Main Content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="h-14 border-b border-border flex items-center px-4 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 flex-shrink-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="mr-2"
          >
            <Menu className="h-4 w-4" />
          </Button>
          <h1 className="text-lg font-semibold">
            Chat with {agentConfig?.name || 'Agent'}
          </h1>
          <div className="ml-auto flex items-center space-x-2">
            <div className="flex items-center space-x-1">
              <div className="h-2 w-2 bg-green-500 rounded-full"></div>
              <span className="text-sm text-muted-foreground">Ready</span>
            </div>
          </div>
        </header>

        {/* Chat Area */}
        <div className="flex-1 overflow-hidden">
          <ChatArea agentConfig={agentConfig} />
        </div>
      </div>
    </div>
  );
}