'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { AgentConfig, SubAgentInfo, Tool } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  ChevronRight,
  ChevronDown,
  X,
  Bot,
  Wrench,
  Brain,
  Settings,
  History,
  Activity,
} from 'lucide-react';
import { MemoryBrowser } from '../memory/memory-browser';

interface SidebarProps {
  agentConfig: AgentConfig | null;
  isOpen: boolean;
  onClose: () => void;
}

export function Sidebar({ agentConfig, isOpen, onClose }: SidebarProps) {
  const [agentInfoOpen, setAgentInfoOpen] = useState(true);
  const [toolsOpen, setToolsOpen] = useState(false);
  const [subAgentsOpen, setSubAgentsOpen] = useState(false);
  const [memoryOpen, setMemoryOpen] = useState(false);
  const [tracesOpen, setTracesOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [memoryBrowserOpen, setMemoryBrowserOpen] = useState(false);

  const [subAgents, setSubAgents] = useState<SubAgentInfo[]>([]);
  const [tools, setTools] = useState<Tool[]>([]);
  const [darkMode, setDarkMode] = useState(false);

  const loadSubAgents = useCallback(async () => {
    try {
      const response = await agentAPI.getSubAgents();
      setSubAgents(response.sub_agents);
    } catch (err) {
      console.error('Failed to load sub-agents:', err);
    }
  }, []);

  const loadTools = useCallback(async () => {
    try {
      const response = await agentAPI.getTools();
      setTools(response.tools);
    } catch (err) {
      console.error('Failed to load tools:', err);
    }
  }, []);

  useEffect(() => {
    if (isOpen) {
      const loadData = async () => {
        await Promise.all([loadSubAgents(), loadTools()]);
      };
      loadData();
    }
  }, [isOpen, loadSubAgents, loadTools]);

  useEffect(() => {
    // Apply dark mode
    if (darkMode) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [darkMode]);

  if (!isOpen) return null;

  return (
    <>
      <div className="h-full flex flex-col bg-background border-r border-border">
        {/* Header */}
        <div className="h-14 border-b border-border flex items-center justify-between px-4">
          <h2 className="font-semibold text-lg">Agent UI</h2>
          <Button variant="ghost" size="icon" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Content */}
        <ScrollArea className="flex-1 p-4">
          <div className="space-y-4">
            {/* Agent Info */}
            <Collapsible open={agentInfoOpen} onOpenChange={setAgentInfoOpen}>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                  <div className="flex items-center space-x-2">
                    <Bot className="h-4 w-4" />
                    <span className="font-medium">Agent Info</span>
                  </div>
                  {agentInfoOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <Card>
                  <CardContent className="p-4 space-y-3">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Name:</label>
                      <p className="text-sm">{agentConfig?.name || 'Unknown'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Model:</label>
                      <p className="text-sm">{agentConfig?.model || 'Unknown'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Description:</label>
                      <p className="text-sm">{agentConfig?.description || 'No description'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">System Prompt:</label>
                      <div className="text-sm p-2 bg-muted rounded-md max-h-20 overflow-y-auto">
                        {agentConfig?.system_prompt || 'No system prompt configured'}
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </CollapsibleContent>
            </Collapsible>

            {/* Tools */}
            <Collapsible open={toolsOpen} onOpenChange={setToolsOpen}>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                  <div className="flex items-center space-x-2">
                    <Wrench className="h-4 w-4" />
                    <span className="font-medium">Tools ({tools.length})</span>
                  </div>
                  {toolsOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <Card>
                  <CardContent className="p-4">
                    {tools.length > 0 ? (
                      <div className="space-y-2">
                        {tools.map((tool, index) => (
                          <div key={index} className="flex items-center justify-between p-2 border rounded-md">
                            <div>
                              <p className="text-sm font-medium">{tool.name}</p>
                              <p className="text-xs text-muted-foreground">{tool.description}</p>
                            </div>
                            <Badge variant={tool.enabled ? 'default' : 'secondary'}>
                              {tool.enabled ? 'Enabled' : 'Disabled'}
                            </Badge>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground">No tools available</p>
                    )}
                  </CardContent>
                </Card>
              </CollapsibleContent>
            </Collapsible>

            {/* Sub-Agents */}
            {subAgents.length > 0 && (
              <Collapsible open={subAgentsOpen} onOpenChange={setSubAgentsOpen}>
                <CollapsibleTrigger asChild>
                  <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                    <div className="flex items-center space-x-2">
                      <Bot className="h-4 w-4" />
                      <span className="font-medium">Sub-Agents ({subAgents.length})</span>
                    </div>
                    {subAgentsOpen ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="mt-2">
                  <div className="space-y-2">
                    {subAgents.map((agent) => (
                      <Card key={agent.id}>
                        <CardHeader className="p-3">
                          <div className="flex justify-between items-start">
                            <CardTitle className="text-sm">{agent.name}</CardTitle>
                            <Badge variant={agent.status === 'active' ? 'default' : 'secondary'}>
                              {agent.status}
                            </Badge>
                          </div>
                        </CardHeader>
                        <CardContent className="p-3 pt-0">
                          <p className="text-xs text-muted-foreground mb-2">{agent.description}</p>
                          <div className="flex gap-1 flex-wrap">
                            <Badge variant="outline" className="text-xs">{agent.model}</Badge>
                            <Badge variant="outline" className="text-xs">{agent.tools.length} tools</Badge>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                </CollapsibleContent>
              </Collapsible>
            )}

            {/* Memory */}
            <Collapsible open={memoryOpen} onOpenChange={setMemoryOpen}>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                  <div className="flex items-center space-x-2">
                    <Brain className="h-4 w-4" />
                    <span className="font-medium">Memory</span>
                  </div>
                  {memoryOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <Card>
                  <CardContent className="p-4 space-y-3">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Type:</label>
                      <p className="text-sm">{agentConfig?.memory.type || 'None'}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">Status:</label>
                      <Badge variant={agentConfig?.memory.status === 'active' ? 'default' : 'secondary'}>
                        {agentConfig?.memory.status || 'Inactive'}
                      </Badge>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      className="w-full"
                      onClick={() => setMemoryBrowserOpen(true)}
                    >
                      <History className="h-4 w-4 mr-2" />
                      View History
                    </Button>
                  </CardContent>
                </Card>
              </CollapsibleContent>
            </Collapsible>

            {/* Traces */}
            {agentConfig?.features.traces && (
              <Collapsible open={tracesOpen} onOpenChange={setTracesOpen}>
                <CollapsibleTrigger asChild>
                  <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                    <div className="flex items-center space-x-2">
                      <Activity className="h-4 w-4" />
                      <span className="font-medium">Traces</span>
                    </div>
                    {tracesOpen ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="mt-2">
                  <Card>
                    <CardContent className="p-4 space-y-3">
                      <div className="text-sm text-muted-foreground">
                        Monitor agent execution traces and performance metrics
                      </div>
                      <Button
                        variant="outline"
                        size="sm"
                        className="w-full"
                        onClick={() => {
                          // TODO: Navigate to traces view
                          console.log('Navigate to traces view');
                        }}
                      >
                        <Activity className="h-4 w-4 mr-2" />
                        View Traces
                      </Button>
                    </CardContent>
                  </Card>
                </CollapsibleContent>
              </Collapsible>
            )}

            {/* Settings */}
            <Collapsible open={settingsOpen} onOpenChange={setSettingsOpen}>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" className="w-full justify-between p-0 h-auto">
                  <div className="flex items-center space-x-2">
                    <Settings className="h-4 w-4" />
                    <span className="font-medium">Settings</span>
                  </div>
                  {settingsOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <Card>
                  <CardContent className="p-4 space-y-4">
                    <div className="flex items-center justify-between">
                      <label htmlFor="dark-mode" className="text-sm font-medium">
                        Dark Mode
                      </label>
                      <Switch
                        id="dark-mode"
                        checked={darkMode}
                        onCheckedChange={setDarkMode}
                      />
                    </div>
                  </CardContent>
                </Card>
              </CollapsibleContent>
            </Collapsible>
          </div>
        </ScrollArea>
      </div>

      {/* Memory Browser Dialog */}
      <MemoryBrowser
        open={memoryBrowserOpen}
        onOpenChange={setMemoryBrowserOpen}
      />
    </>
  );
}
