'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { UITrace, TracesResponse, TraceStats } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { TraceDetailViewer } from '../traces/trace-detail-viewer';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
// Using native select for simplicity
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Activity,
  Clock,
  AlertCircle,
  CheckCircle,
  Trash2,
  Eye,
  RefreshCw,
  Search,
  Filter,
} from 'lucide-react';

export function TracesScreen() {
  const [traces, setTraces] = useState<UITrace[]>([]);
  const [stats, setStats] = useState<TraceStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedTrace, setSelectedTrace] = useState<UITrace | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [sortBy, setSortBy] = useState<'start_time' | 'duration_ms' | 'name'>('start_time');
  const [pagination, setPagination] = useState({
    limit: 25,
    offset: 0,
    total: 0,
  });

  const loadTraces = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      const [tracesResponse, statsResponse] = await Promise.all([
        agentAPI.getTraces(pagination.limit, pagination.offset),
        agentAPI.getTraceStats(),
      ]);

      setTraces(tracesResponse.traces);
      setPagination(prev => ({
        ...prev,
        total: tracesResponse.total,
      }));
      setStats(statsResponse);
    } catch (err) {
      console.error('Failed to load traces:', err);
      setError(err instanceof Error ? err.message : 'Failed to load traces');
    } finally {
      setLoading(false);
    }
  }, [pagination.limit, pagination.offset]);

  const deleteTrace = async (traceId: string) => {
    try {
      await agentAPI.deleteTrace(traceId);
      await loadTraces(); // Refresh the list
    } catch (err) {
      console.error('Failed to delete trace:', err);
      setError(err instanceof Error ? err.message : 'Failed to delete trace');
    }
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

  const filteredAndSortedTraces = traces
    .filter(trace => {
      const matchesSearch = searchQuery === '' ||
        trace.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        trace.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (trace.conversation_id && trace.conversation_id.toLowerCase().includes(searchQuery.toLowerCase()));

      const matchesStatus = statusFilter === 'all' || trace.status === statusFilter;

      return matchesSearch && matchesStatus;
    })
    .sort((a, b) => {
      switch (sortBy) {
        case 'start_time':
          return new Date(b.start_time).getTime() - new Date(a.start_time).getTime();
        case 'duration_ms':
          return b.duration_ms - a.duration_ms;
        case 'name':
          return a.name.localeCompare(b.name);
        default:
          return 0;
      }
    });

  useEffect(() => {
    loadTraces();
  }, [loadTraces]);

  // Auto-refresh every 10 seconds for running traces
  useEffect(() => {
    const interval = setInterval(() => {
      if (traces.some(trace => trace.status === 'running')) {
        loadTraces();
      }
    }, 10000);

    return () => clearInterval(interval);
  }, [traces, loadTraces]);

  const handleNextPage = () => {
    if (pagination.offset + pagination.limit < pagination.total) {
      setPagination(prev => ({
        ...prev,
        offset: prev.offset + prev.limit,
      }));
    }
  };

  const handlePrevPage = () => {
    if (pagination.offset > 0) {
      setPagination(prev => ({
        ...prev,
        offset: Math.max(0, prev.offset - prev.limit),
      }));
    }
  };

  if (loading && traces.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-6 w-6 animate-spin mr-2" />
        <span>Loading traces...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold">Agent Traces</h2>
          <p className="text-muted-foreground">
            Monitor and analyze agent execution traces
          </p>
        </div>
        <Button onClick={loadTraces} disabled={loading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {error && (
        <Card className="border-red-200 bg-red-50">
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2 text-red-600">
              <AlertCircle className="h-4 w-4" />
              <span>{error}</span>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Total Traces</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stats.total_traces}</div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Running</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-blue-600">{stats.running_traces}</div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Error Rate</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-red-600">
                {(stats.error_rate * 100).toFixed(1)}%
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Avg Duration</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {formatDuration(stats.avg_duration_ms)}
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Filters */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Filters</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col md:flex-row gap-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search traces..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-8"
                />
              </div>
            </div>
            <div className="w-40">
              <select
                className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value)}
              >
                <option value="all">All Status</option>
                <option value="running">Running</option>
                <option value="completed">Completed</option>
                <option value="error">Error</option>
              </select>
            </div>
            <div className="w-40">
              <select
                className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm"
                value={sortBy}
                onChange={(e) => setSortBy(e.target.value as any)}
              >
                <option value="start_time">Start Time</option>
                <option value="duration_ms">Duration</option>
                <option value="name">Name</option>
              </select>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Traces Table */}
      <Card>
        <CardHeader>
          <CardTitle>Traces</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Duration</TableHead>
                  <TableHead>Spans</TableHead>
                  <TableHead>Start Time</TableHead>
                  <TableHead>Conversation</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredAndSortedTraces.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="text-center py-8">
                      <div className="flex flex-col items-center space-y-2">
                        <Activity className="h-8 w-8 text-muted-foreground" />
                        <span className="text-muted-foreground">No traces found</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredAndSortedTraces.map((trace) => (
                    <TableRow key={trace.id}>
                      <TableCell>
                        <div className="flex items-center space-x-2">
                          {getStatusIcon(trace.status)}
                          <Badge variant={getStatusBadgeVariant(trace.status)}>
                            {trace.status}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="font-medium">{trace.name}</div>
                        <div className="text-sm text-muted-foreground">
                          {trace.id.slice(0, 8)}...
                        </div>
                      </TableCell>
                      <TableCell>
                        {formatDuration(trace.duration_ms)}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{trace.spans.length}</Badge>
                      </TableCell>
                      <TableCell>
                        <div className="text-sm">
                          {formatTimestamp(trace.start_time)}
                        </div>
                      </TableCell>
                      <TableCell>
                        {trace.conversation_id ? (
                          <Badge variant="secondary">
                            {trace.conversation_id.slice(0, 8)}...
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center space-x-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedTrace(trace)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => deleteTrace(trace.id)}
                            className="text-red-600 hover:text-red-700"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between mt-4">
            <div className="text-sm text-muted-foreground">
              Showing {Math.min(pagination.offset + 1, pagination.total)} to{' '}
              {Math.min(pagination.offset + pagination.limit, pagination.total)} of{' '}
              {pagination.total} traces
            </div>
            <div className="flex items-center space-x-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handlePrevPage}
                disabled={pagination.offset === 0}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleNextPage}
                disabled={pagination.offset + pagination.limit >= pagination.total}
              >
                Next
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Trace Detail Modal (placeholder) */}
      {selectedTrace && (
        <TraceDetailModal
          trace={selectedTrace}
          onClose={() => setSelectedTrace(null)}
        />
      )}
    </div>
  );
}

// Use the detailed trace viewer
function TraceDetailModal({ trace, onClose }: { trace: UITrace; onClose: () => void }) {
  return <TraceDetailViewer trace={trace} onClose={onClose} />;
}