'use client';

import React, { useState, useEffect } from 'react';
import { agentAPI } from '@/lib/api';
import { MemoryEntry } from '@/types/agent';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Database, Search, MessageSquare, User, Bot, RefreshCw, ChevronLeft, ChevronRight } from 'lucide-react';

export function MemoryScreen() {
  const [entries, setEntries] = useState<MemoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<MemoryEntry[]>([]);
  const [searching, setSearching] = useState(false);
  const [total, setTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [limit] = useState(20);

  useEffect(() => {
    loadMemory();
  }, [currentPage]);

  const loadMemory = async () => {
    try {
      setLoading(true);
      const offset = (currentPage - 1) * limit;
      const response = await agentAPI.getMemory(limit, offset);
      setEntries(response.entries);
      setTotal(response.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load memory');
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = async () => {
    if (!searchQuery.trim()) {
      setSearchResults([]);
      return;
    }

    try {
      setSearching(true);
      const response = await agentAPI.searchMemory(searchQuery);
      setSearchResults(response.results);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to search memory');
    } finally {
      setSearching(false);
    }
  };

  const clearSearch = () => {
    setSearchQuery('');
    setSearchResults([]);
  };

  const formatTimestamp = (timestamp: number) => {
    return new Date(timestamp).toLocaleString();
  };

  const getRoleIcon = (role: string) => {
    switch (role.toLowerCase()) {
      case 'user':
        return <User className="h-4 w-4" />;
      case 'assistant':
        return <Bot className="h-4 w-4" />;
      default:
        return <MessageSquare className="h-4 w-4" />;
    }
  };

  const getRoleBadgeVariant = (role: string) => {
    switch (role.toLowerCase()) {
      case 'user':
        return 'default';
      case 'assistant':
        return 'secondary';
      default:
        return 'outline';
    }
  };

  const displayEntries = searchQuery && searchResults.length > 0 ? searchResults : entries;
  const totalPages = Math.ceil(total / limit);

  if (loading && entries.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="animate-spin rounded-full h-16 w-16 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-lg">Loading memory...</p>
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
            <Database className="h-6 w-6" />
            <h1 className="text-2xl font-bold">Memory Browser</h1>
            <Badge variant="secondary">{total} entries</Badge>
          </div>
          <Button variant="outline" size="sm" onClick={loadMemory} disabled={loading}>
            <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
        </div>

        {/* Search */}
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search memory entries..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              className="pl-10"
            />
          </div>
          <Button onClick={handleSearch} disabled={searching || !searchQuery.trim()}>
            {searching ? 'Searching...' : 'Search'}
          </Button>
          {searchQuery && (
            <Button variant="outline" onClick={clearSearch}>
              Clear
            </Button>
          )}
        </div>

        {searchQuery && searchResults.length > 0 && (
          <div className="mt-2">
            <Badge variant="outline">
              {searchResults.length} search results for "{searchQuery}"
            </Badge>
          </div>
        )}
      </div>

      {/* Memory Entries */}
      <ScrollArea className="flex-1">
        <div className="p-6">
          {error && (
            <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg">
              <p className="text-red-700">{error}</p>
            </div>
          )}

          {displayEntries.length === 0 ? (
            <div className="text-center py-12">
              <Database className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
              <p className="text-lg text-muted-foreground mb-2">
                {searchQuery ? 'No memory entries found' : 'No memory entries available'}
              </p>
              {searchQuery && (
                <p className="text-sm text-muted-foreground">
                  Try adjusting your search terms
                </p>
              )}
            </div>
          ) : (
            <div className="space-y-4">
              {displayEntries.map((entry) => (
                <Card key={entry.id} className="hover:shadow-md transition-shadow">
                  <CardHeader className="pb-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        {getRoleIcon(entry.role)}
                        <Badge variant={getRoleBadgeVariant(entry.role)}>
                          {entry.role}
                        </Badge>
                        {entry.conversation_id && entry.conversation_id !== 'default' && (
                          <Badge variant="outline" className="text-xs">
                            {entry.conversation_id}
                          </Badge>
                        )}
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {formatTimestamp(entry.timestamp)}
                      </span>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <div className="prose prose-sm max-w-none">
                      <p className="whitespace-pre-wrap text-sm">{entry.content}</p>
                    </div>
                    {entry.metadata && Object.keys(entry.metadata).length > 0 && (
                      <details className="mt-3">
                        <summary className="text-xs text-muted-foreground cursor-pointer">
                          Metadata
                        </summary>
                        <pre className="text-xs bg-muted p-2 rounded mt-1 overflow-auto">
                          {JSON.stringify(entry.metadata, null, 2)}
                        </pre>
                      </details>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </div>
      </ScrollArea>

      {/* Pagination */}
      {!searchQuery && totalPages > 1 && (
        <div className="p-6 border-t border-border">
          <div className="flex items-center justify-between">
            <div className="text-sm text-muted-foreground">
              Page {currentPage} of {totalPages} • {total} total entries
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(prev => Math.max(1, prev - 1))}
                disabled={currentPage === 1 || loading}
              >
                <ChevronLeft className="h-4 w-4 mr-1" />
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(prev => Math.min(totalPages, prev + 1))}
                disabled={currentPage === totalPages || loading}
              >
                Next
                <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}