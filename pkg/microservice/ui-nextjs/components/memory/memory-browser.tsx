'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { MemoryEntry } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Search, Download, RefreshCw, Loader2 } from 'lucide-react';

interface MemoryBrowserProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function MemoryBrowser({ open, onOpenChange }: MemoryBrowserProps) {
  const [entries, setEntries] = useState<MemoryEntry[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<MemoryEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [searching, setSearching] = useState(false);
  const [pagination, setPagination] = useState({
    limit: 50,
    offset: 0,
    total: 0,
  });

  const loadMemory = useCallback(async () => {
    try {
      setLoading(true);
      const response = await agentAPI.getMemory(pagination.limit, pagination.offset);
      // Handle both old and new response formats
      if (response.mode === 'messages' && response.messages) {
        setEntries(response.messages);
      } else if ('entries' in response) {
        // Fallback for old format
        setEntries((response as any).entries || []);
      }
      setPagination(prev => ({
        ...prev,
        total: response.total,
      }));
    } catch (err) {
      console.error('Failed to load memory:', err);
    } finally {
      setLoading(false);
    }
  }, [pagination.limit, pagination.offset]);

  const performSearch = useCallback(async () => {
    if (!searchQuery.trim()) return;

    try {
      setSearching(true);
      const response = await agentAPI.searchMemory(searchQuery);
      setSearchResults(response.results);
    } catch (err) {
      console.error('Failed to search memory:', err);
    } finally {
      setSearching(false);
    }
  }, [searchQuery]);

  useEffect(() => {
    if (open) {
      loadMemory();
    }
  }, [open, loadMemory]);

  useEffect(() => {
    const delayedSearch = setTimeout(() => {
      if (searchQuery.trim()) {
        performSearch();
      } else {
        setSearchResults([]);
      }
    }, 300);

    return () => clearTimeout(delayedSearch);
  }, [searchQuery, performSearch]);

  const loadMore = async () => {
    try {
      setLoading(true);
      const newOffset = pagination.offset + pagination.limit;
      const response = await agentAPI.getMemory(pagination.limit, newOffset);
      // Handle both old and new response formats
      let newEntries: MemoryEntry[] = [];
      if (response.mode === 'messages' && response.messages) {
        newEntries = response.messages;
      } else if ('entries' in response) {
        // Fallback for old format
        newEntries = (response as any).entries || [];
      }
      setEntries(prev => [...prev, ...newEntries]);
      setPagination(prev => ({
        ...prev,
        offset: newOffset,
      }));
    } catch (err) {
      console.error('Failed to load more memory:', err);
    } finally {
      setLoading(false);
    }
  };

  const exportMemory = () => {
    const dataToExport = searchQuery.trim() ? searchResults : entries;
    const dataStr = JSON.stringify(dataToExport, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });
    const url = URL.createObjectURL(dataBlob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `memory-export-${new Date().toISOString().split('T')[0]}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const formatTimestamp = (timestamp: number) => {
    return new Date(timestamp).toLocaleString();
  };

  const formatRole = (role: string) => {
    return role.charAt(0).toUpperCase() + role.slice(1);
  };

  const displayEntries = searchQuery.trim() ? searchResults : entries;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Conversation History</DialogTitle>
        </DialogHeader>

        {/* Controls */}
        <div className="flex items-center space-x-2 pb-4 border-b">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search conversation history..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10"
            />
            {searching && (
              <Loader2 className="absolute right-3 top-3 h-4 w-4 animate-spin text-muted-foreground" />
            )}
          </div>
          <Button variant="outline" onClick={loadMemory} disabled={loading}>
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
          <Button variant="outline" onClick={exportMemory} disabled={displayEntries.length === 0}>
            <Download className="h-4 w-4" />
          </Button>
        </div>

        {/* Results */}
        <ScrollArea className="flex-1">
          {displayEntries.length === 0 ? (
            <div className="flex items-center justify-center h-32">
              <p className="text-muted-foreground">
                {loading ? 'Loading...' : searchQuery.trim() ? 'No search results found' : 'No conversation history'}
              </p>
            </div>
          ) : (
            <div className="space-y-3 p-1">
              {displayEntries.map((entry) => (
                <Card key={entry.id} className="border">
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex items-center space-x-2">
                        <Badge variant={entry.role === 'user' ? 'default' : 'secondary'}>
                          {formatRole(entry.role)}
                        </Badge>
                        {entry.conversation_id && (
                          <Badge variant="outline" className="text-xs">
                            {entry.conversation_id}
                          </Badge>
                        )}
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {formatTimestamp(entry.timestamp)}
                      </span>
                    </div>
                    <div className="text-sm">
                      <pre className="whitespace-pre-wrap font-sans leading-relaxed">
                        {entry.content.length > 500
                          ? `${entry.content.substring(0, 500)}...`
                          : entry.content}
                      </pre>
                    </div>
                    {entry.metadata && Object.keys(entry.metadata).length > 0 && (
                      <div className="mt-2 pt-2 border-t text-xs text-muted-foreground">
                        <strong>Metadata:</strong> {JSON.stringify(entry.metadata)}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}

              {/* Load More Button */}
              {!searchQuery.trim() && pagination.offset + pagination.limit < pagination.total && (
                <div className="flex justify-center pt-4">
                  <Button variant="outline" onClick={loadMore} disabled={loading}>
                    {loading ? (
                      <Loader2 className="h-4 w-4 animate-spin mr-2" />
                    ) : null}
                    Load More ({pagination.total - (pagination.offset + pagination.limit)} remaining)
                  </Button>
                </div>
              )}
            </div>
          )}
        </ScrollArea>

        {/* Footer */}
        <div className="pt-4 border-t">
          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <span>
              {searchQuery.trim()
                ? `${searchResults.length} search results`
                : `${entries.length} of ${pagination.total} entries loaded`}
            </span>
            <span>
              {searchQuery.trim() && `for "${searchQuery}"`}
            </span>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
