'use client';

import React, { useState } from 'react';
import { UITrace, UITraceSpan } from '@/types/agent';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import {
  Activity,
  Clock,
  AlertCircle,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  X,
  Code,
  ArrowRight,
  Zap,
  MessageCircle,
} from 'lucide-react';

interface TraceDetailViewerProps {
  trace: UITrace;
  onClose: () => void;
}

type SpanWithChildren = UITraceSpan & {
  children: SpanWithChildren[];
};

export function TraceDetailViewer({ trace, onClose }: TraceDetailViewerProps) {
  const [expandedSpans, setExpandedSpans] = useState<Set<string>>(new Set());
  const [selectedTab, setSelectedTab] = useState<'timeline' | 'spans' | 'raw'>('timeline');

  const toggleSpanExpansion = (spanId: string) => {
    const newExpanded = new Set(expandedSpans);
    if (newExpanded.has(spanId)) {
      newExpanded.delete(spanId);
    } else {
      newExpanded.add(spanId);
    }
    setExpandedSpans(newExpanded);
  };

  const formatDuration = (ms: number): string => {
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    return `${(ms / 60000).toFixed(1)}m`;
  };

  const formatTimestamp = (timestamp: string): string => {
    return new Date(timestamp).toLocaleString();
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'error':
        return <AlertCircle className="h-4 w-4 text-red-500" />;
      case 'running':
        return <Activity className="h-4 w-4 text-blue-500 animate-pulse" />;
      default:
        return <Clock className="h-4 w-4 text-gray-500" />;
    }
  };

  const getSpanTypeIcon = (type: string) => {
    switch (type) {
      case 'generation':
        return <MessageCircle className="h-4 w-4 text-blue-500" />;
      case 'tool_call':
        return <Zap className="h-4 w-4 text-yellow-500" />;
      case 'event':
        return <Activity className="h-4 w-4 text-green-500" />;
      default:
        return <Code className="h-4 w-4 text-gray-500" />;
    }
  };

  const getStatusBadgeVariant = (status: string) => {
    switch (status) {
      case 'completed':
        return 'default';
      case 'error':
        return 'destructive';
      case 'running':
        return 'secondary';
      default:
        return 'outline';
    }
  };

  const buildSpanHierarchy = (spans: UITraceSpan[]) => {
    const spanMap = new Map<string, SpanWithChildren>();
    const rootSpans: SpanWithChildren[] = [];

    // Initialize all spans with empty children array
    spans.forEach(span => {
      spanMap.set(span.id, { ...span, children: [] });
    });

    // Build hierarchy
    spans.forEach(span => {
      const spanWithChildren = spanMap.get(span.id)!;
      if (span.parent_id && spanMap.has(span.parent_id)) {
        spanMap.get(span.parent_id)!.children.push(spanWithChildren);
      } else {
        rootSpans.push(spanWithChildren);
      }
    });

    return rootSpans;
  };

  const renderSpanTree = (spans: SpanWithChildren[], level = 0) => {
    return spans.map((span) => (
      <div key={span.id} style={{ marginLeft: `${level * 20}px` }}>
        <Collapsible
          open={expandedSpans.has(span.id)}
          onOpenChange={() => toggleSpanExpansion(span.id)}
        >
          <CollapsibleTrigger asChild>
            <Card className="mb-2 cursor-pointer hover:bg-muted/50">
              <CardContent className="p-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-3">
                    {span.children.length > 0 && (
                      expandedSpans.has(span.id) ? (
                        <ChevronDown className="h-4 w-4" />
                      ) : (
                        <ChevronRight className="h-4 w-4" />
                      )
                    )}
                    {getSpanTypeIcon(span.type)}
                    <div>
                      <div className="font-medium">{span.name}</div>
                      <div className="text-sm text-muted-foreground">
                        {formatTimestamp(span.start_time)} • {formatDuration(span.duration_ms)}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center space-x-2">
                    <Badge variant="outline">{span.type}</Badge>
                    {span.error && (
                      <Badge variant="destructive">Error</Badge>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="ml-6 mb-4">
              {(() => {
                const hasContent = span.input ||
                                 span.output ||
                                 span.error ||
                                 (span.events && span.events.length > 0) ||
                                 (span.attributes && Object.keys(span.attributes).length > 0);

                if (!hasContent) return null;

                return (
                  <Card>
                    <CardContent className="p-4 space-y-4">
                      {span.input && (
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">Input:</label>
                          <div className="text-sm p-2 bg-muted rounded-md mt-1 max-h-20 overflow-y-auto">
                            {span.input}
                          </div>
                        </div>
                      )}

                      {span.output && (
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">Output:</label>
                          <div className="text-sm p-2 bg-muted rounded-md mt-1 max-h-20 overflow-y-auto">
                            {span.output}
                          </div>
                        </div>
                      )}

                      {span.error && (
                        <div>
                          <label className="text-sm font-medium text-destructive">Error:</label>
                          <div className="text-sm p-2 bg-destructive/10 border border-destructive/20 rounded-md mt-1">
                            <div className="font-medium">{span.error.message}</div>
                            {span.error.type && (
                              <div className="text-xs text-muted-foreground mt-1">
                                Type: {span.error.type}
                              </div>
                            )}
                          </div>
                        </div>
                      )}

                      {span.events && span.events.length > 0 && (
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">Events:</label>
                          <div className="space-y-1 mt-1">
                            {span.events.map((event, idx) => (
                              <div key={idx} className="text-sm p-2 bg-muted rounded-md">
                                <div className="font-medium">{event.name}</div>
                                <div className="text-xs text-muted-foreground">
                                  {formatTimestamp(event.timestamp)}
                                </div>
                                {event.attributes && Object.keys(event.attributes).length > 0 && (
                                  <div className="text-xs mt-1">
                                    {JSON.stringify(event.attributes)}
                                  </div>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {span.attributes && Object.keys(span.attributes).length > 0 && (
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">Attributes:</label>
                          <div className="space-y-2 mt-1">
                            {Object.entries(span.attributes).map(([key, value]) => {
                              const stringValue = String(value);
                              const isLongContent = stringValue.length > 500;

                              return (
                                <div key={key} className="border rounded-md p-2 bg-muted">
                                  <div className="text-xs font-medium text-muted-foreground mb-1">{key}:</div>
                                  <div className="text-xs">
                                    {isLongContent ? (
                                      <details className="cursor-pointer">
                                        <summary className="text-blue-600 hover:text-blue-800">
                                          {stringValue.slice(0, 100)}... (click to expand {stringValue.length} chars)
                                        </summary>
                                        <div className="mt-2 p-2 bg-background rounded border max-h-40 overflow-y-auto">
                                          <pre className="whitespace-pre-wrap text-xs">{stringValue}</pre>
                                        </div>
                                      </details>
                                    ) : (
                                      <span className="break-words">{stringValue}</span>
                                    )}
                                  </div>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      )}
                    </CardContent>
                  </Card>
                );
              })()}
            </div>
            {span.children.length > 0 && renderSpanTree(span.children, level + 1)}
          </CollapsibleContent>
        </Collapsible>
      </div>
    ));
  };

  const spanHierarchy = buildSpanHierarchy(trace.spans);

  return (
    <div
      className="fixed inset-0 bg-transparent flex items-center justify-center p-4 z-50"
      onClick={onClose}
    >
      <Card
        className="max-w-6xl w-full h-[90vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <CardHeader className="border-b">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-3">
              {getStatusIcon(trace.status)}
              <div>
                <CardTitle>{trace.name}</CardTitle>
                <div className="text-sm text-muted-foreground">
                  {formatTimestamp(trace.start_time)} • {formatDuration(trace.duration_ms)}
                </div>
              </div>
            </div>
            <div className="flex items-center space-x-2">
              <Badge variant={getStatusBadgeVariant(trace.status)}>
                {trace.status}
              </Badge>
              <Button variant="ghost" size="icon" onClick={onClose}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardHeader>

        {/* Tab Navigation */}
        <div className="border-b">
          <nav className="flex space-x-8 px-6">
            {[
              { id: 'timeline' as const, label: 'Timeline', icon: Clock },
              { id: 'spans' as const, label: 'Spans', icon: Activity },
              { id: 'raw' as const, label: 'Raw Data', icon: Code },
            ].map((tab) => (
              <button
                key={tab.id}
                onClick={() => setSelectedTab(tab.id)}
                className={`flex items-center space-x-2 py-4 border-b-2 transition-colors ${
                  selectedTab === tab.id
                    ? 'border-primary text-primary'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <tab.icon className="h-4 w-4" />
                <span>{tab.label}</span>
              </button>
            ))}
          </nav>
        </div>

        {/* Tab Content */}
        <CardContent className="flex-1 p-0 overflow-hidden">
          <ScrollArea className="h-full">
            <div className="p-6">
              {selectedTab === 'timeline' && (
                <div className="space-y-4">
                  {(() => {
                    const toolCallsCount = trace.spans.filter(s => s.type === 'tool_call').length;
                    const errorsCount = trace.spans.filter(s => s.error).length;
                    const hasData = toolCallsCount > 0 || errorsCount > 0;

                    if (!hasData) return null;

                    return (
                      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
                        {trace.spans.length > 0 && (
                          <Card>
                            <CardContent className="pt-6">
                              <div className="text-2xl font-bold">{trace.spans.length}</div>
                              <div className="text-sm text-muted-foreground">Total Spans</div>
                            </CardContent>
                          </Card>
                        )}
                        {toolCallsCount > 0 && (
                          <Card>
                            <CardContent className="pt-6">
                              <div className="text-2xl font-bold">{toolCallsCount}</div>
                              <div className="text-sm text-muted-foreground">Tool Calls</div>
                            </CardContent>
                          </Card>
                        )}
                        {errorsCount > 0 && (
                          <Card>
                            <CardContent className="pt-6">
                              <div className="text-2xl font-bold">{errorsCount}</div>
                              <div className="text-sm text-muted-foreground">Errors</div>
                            </CardContent>
                          </Card>
                        )}
                      </div>
                    );
                  })()}

                  <div>
                    <h3 className="text-lg font-semibold mb-4">Execution Timeline</h3>
                    {renderSpanTree(spanHierarchy)}
                  </div>
                </div>
              )}

              {selectedTab === 'spans' && (
                <div className="space-y-4">
                  <h3 className="text-lg font-semibold">All Spans</h3>
                  {trace.spans.map((span) => (
                    <Card key={span.id}>
                      <CardHeader>
                        <div className="flex items-center justify-between">
                          <div className="flex items-center space-x-3">
                            {getSpanTypeIcon(span.type)}
                            <div>
                              <CardTitle className="text-base">{span.name}</CardTitle>
                              <div className="text-sm text-muted-foreground">
                                ID: {span.id}
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center space-x-2">
                            <Badge variant="outline">{span.type}</Badge>
                            <Badge variant="outline">{formatDuration(span.duration_ms)}</Badge>
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent>
                        <div className="grid grid-cols-2 gap-4 text-sm">
                          <div>
                            <strong>Start:</strong> {formatTimestamp(span.start_time)}
                          </div>
                          <div>
                            <strong>Duration:</strong> {formatDuration(span.duration_ms)}
                          </div>
                          {span.parent_id && (
                            <div>
                              <strong>Parent:</strong> {span.parent_id.slice(0, 8)}...
                            </div>
                          )}
                          {span.error && (
                            <div className="col-span-2">
                              <strong className="text-destructive">Error:</strong> {span.error.message}
                            </div>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}

              {selectedTab === 'raw' && (
                <div>
                  <h3 className="text-lg font-semibold mb-4">Raw Trace Data</h3>
                  <Card>
                    <CardContent className="pt-6">
                      <ScrollArea className="h-96">
                        <pre className="text-sm whitespace-pre-wrap">
                          {JSON.stringify(trace, null, 2)}
                        </pre>
                      </ScrollArea>
                    </CardContent>
                  </Card>
                </div>
              )}
            </div>
          </ScrollArea>
        </CardContent>
      </Card>
    </div>
  );
}
