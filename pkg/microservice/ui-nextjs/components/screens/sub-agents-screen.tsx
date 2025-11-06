'use client';

import React, { useState, useEffect } from 'react';
import { agentAPI } from '@/lib/api';
import { SubAgentInfo } from '@/types/agent';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Users, Search, Send, Bot, Wrench, CheckCircle, AlertCircle, RefreshCw, Plus } from 'lucide-react';

export function SubAgentsScreen() {
  const [subAgents, setSubAgents] = useState<SubAgentInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [delegateDialogOpen, setDelegateDialogOpen] = useState(false);
  const [selectedSubAgent, setSelectedSubAgent] = useState<SubAgentInfo | null>(null);
  const [delegateTask, setDelegateTask] = useState('');
  const [delegating, setDelegating] = useState(false);
  const [delegateResult, setDelegateResult] = useState<string | null>(null);

  useEffect(() => {
    loadSubAgents();
  }, []);

  const loadSubAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await agentAPI.getSubAgents();
      setSubAgents(response.sub_agents);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sub-agents');
    } finally {
      setLoading(false);
    }
  };

  const handleDelegate = async () => {
    if (!selectedSubAgent || !delegateTask.trim()) return;

    try {
      setDelegating(true);
      const response = await agentAPI.delegateToSubAgent({
        sub_agent_id: selectedSubAgent.id,
        task: delegateTask,
        context: {},
      });
      setDelegateResult(response.result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delegate task');
    } finally {
      setDelegating(false);
    }
  };

  const openDelegateDialog = (subAgent: SubAgentInfo) => {
    setSelectedSubAgent(subAgent);
    setDelegateTask('');
    setDelegateResult(null);
    setDelegateDialogOpen(true);
  };

  const closeDelegateDialog = () => {
    setDelegateDialogOpen(false);
    setSelectedSubAgent(null);
    setDelegateTask('');
    setDelegateResult(null);
  };

  const filteredSubAgents = subAgents.filter(agent =>
    agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    agent.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
    agent.model.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const getStatusIcon = (status: string) => {
    switch (status.toLowerCase()) {
      case 'active':
      case 'ready':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'inactive':
      case 'offline':
        return <AlertCircle className="h-4 w-4 text-red-500" />;
      default:
        return <AlertCircle className="h-4 w-4 text-yellow-500" />;
    }
  };

  const getStatusBadgeVariant = (status: string) => {
    switch (status.toLowerCase()) {
      case 'active':
      case 'ready':
        return 'default';
      case 'inactive':
      case 'offline':
        return 'destructive';
      default:
        return 'secondary';
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="animate-spin rounded-full h-16 w-16 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-lg">Loading sub-agents...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="p-6 border-b border-border">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <Users className="h-6 w-6" />
            <h1 className="text-2xl font-bold">Sub-Agents</h1>
            <Badge variant="secondary">{subAgents.length} available</Badge>
          </div>
          <Button variant="outline" size="sm" onClick={loadSubAgents} disabled={loading}>
            <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
        </div>

        {/* Search */}
        <div className="relative">
          <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search sub-agents..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>
      </div>

      {/* Sub-Agents List */}
      <ScrollArea className="flex-1">
        <div className="p-6">
          {error && (
            <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg">
              <p className="text-red-700">{error}</p>
            </div>
          )}

          {filteredSubAgents.length === 0 ? (
            <div className="text-center py-12">
              <Users className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
              <p className="text-lg text-muted-foreground mb-2">
                {searchQuery ? 'No sub-agents found' : 'No sub-agents available'}
              </p>
              {searchQuery ? (
                <p className="text-sm text-muted-foreground">
                  Try adjusting your search terms
                </p>
              ) : (
                <div className="space-y-2">
                  <p className="text-sm text-muted-foreground">
                    Sub-agents are specialized AI assistants that can handle specific tasks
                  </p>
                  <Button variant="outline" size="sm" className="mt-4">
                    <Plus className="h-4 w-4 mr-2" />
                    Create Sub-Agent
                  </Button>
                </div>
              )}
            </div>
          ) : (
            <div className="grid gap-4">
              {filteredSubAgents.map((subAgent) => (
                <Card key={subAgent.id} className="hover:shadow-md transition-shadow">
                  <CardHeader className="pb-3">
                    <CardTitle className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <Bot className="h-5 w-5" />
                        <span>{subAgent.name}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        {getStatusIcon(subAgent.status)}
                        <Badge variant={getStatusBadgeVariant(subAgent.status)}>
                          {subAgent.status}
                        </Badge>
                      </div>
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <p className="text-sm text-muted-foreground">
                      {subAgent.description}
                    </p>

                    {/* Model and Tools Info */}
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="outline">
                        Model: {subAgent.model}
                      </Badge>
                      <Badge variant="outline">
                        <Wrench className="h-3 w-3 mr-1" />
                        {subAgent.tools.length} tools
                      </Badge>
                      {subAgent.capabilities && subAgent.capabilities.length > 0 && (
                        <Badge variant="outline">
                          {subAgent.capabilities.length} capabilities
                        </Badge>
                      )}
                    </div>

                    {/* Tools List */}
                    {subAgent.tools && subAgent.tools.length > 0 && (
                      <details className="text-sm">
                        <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                          Available Tools ({subAgent.tools.length})
                        </summary>
                        <div className="mt-2 pl-4 space-y-1">
                          {subAgent.tools.map((tool, index) => (
                            <div key={index} className="flex items-center gap-2">
                              <Wrench className="h-3 w-3" />
                              <code className="text-xs bg-muted px-1 rounded">{tool}</code>
                            </div>
                          ))}
                        </div>
                      </details>
                    )}

                    {/* Capabilities List */}
                    {subAgent.capabilities && subAgent.capabilities.length > 0 && (
                      <details className="text-sm">
                        <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                          Capabilities ({subAgent.capabilities.length})
                        </summary>
                        <div className="mt-2 pl-4 space-y-1">
                          {subAgent.capabilities.map((capability, index) => (
                            <div key={index} className="text-xs">
                              • {capability}
                            </div>
                          ))}
                        </div>
                      </details>
                    )}

                    {/* Actions */}
                    <div className="flex justify-end pt-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => openDelegateDialog(subAgent)}
                        disabled={subAgent.status.toLowerCase() !== 'active' && subAgent.status.toLowerCase() !== 'ready'}
                      >
                        <Send className="h-4 w-4 mr-2" />
                        Delegate Task
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </div>
      </ScrollArea>

      {/* Delegate Task Dialog */}
      <Dialog open={delegateDialogOpen} onOpenChange={setDelegateDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Send className="h-5 w-5" />
              Delegate Task to {selectedSubAgent?.name}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {selectedSubAgent && (
              <div className="p-4 bg-muted/50 rounded-lg">
                <div className="flex items-center justify-between mb-2">
                  <span className="font-medium">{selectedSubAgent.name}</span>
                  <Badge variant="outline">{selectedSubAgent.model}</Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  {selectedSubAgent.description}
                </p>
              </div>
            )}

            <div className="space-y-2">
              <label className="text-sm font-medium">Task Description</label>
              <Textarea
                placeholder="Describe the task you want to delegate to this sub-agent..."
                value={delegateTask}
                onChange={(e) => setDelegateTask(e.target.value)}
                rows={4}
                disabled={delegating}
              />
            </div>

            {delegateResult && (
              <div className="space-y-2">
                <label className="text-sm font-medium">Result</label>
                <div className="p-4 bg-muted/50 rounded-lg">
                  <p className="text-sm">{delegateResult}</p>
                </div>
              </div>
            )}

            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={closeDelegateDialog} disabled={delegating}>
                {delegateResult ? 'Close' : 'Cancel'}
              </Button>
              {!delegateResult && (
                <Button onClick={handleDelegate} disabled={!delegateTask.trim() || delegating}>
                  {delegating ? (
                    <>
                      <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
                      Delegating...
                    </>
                  ) : (
                    <>
                      <Send className="h-4 w-4 mr-2" />
                      Delegate Task
                    </>
                  )}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}