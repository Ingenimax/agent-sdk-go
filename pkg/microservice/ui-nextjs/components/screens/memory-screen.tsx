'use client';

import React, { useState, useEffect } from 'react';
import { agentAPI } from '@/lib/api';
import { MemoryEntry, MemoryResponse, ConversationInfo } from '@/types/agent';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Database, Search, MessageSquare, User, Bot, RefreshCw, ChevronLeft, ChevronRight } from 'lucide-react';

export function MemoryScreen() {
  // State for conversation list (left panel)
  const [conversations, setConversations] = useState<ConversationInfo[]>([]);
  const [conversationsLoading, setConversationsLoading] = useState(true);

  // State for conversation detail (right panel)
  const [selectedConversationId, setSelectedConversationId] = useState<string | null>(null);
  const [messages, setMessages] = useState<MemoryEntry[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [messagesTotal, setMessagesTotal] = useState(0);
  const [messagesOffset, setMessagesOffset] = useState(0);
  const [messagesLimit] = useState(50);

  // Common state
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  // Load conversations on mount
  useEffect(() => {
    loadConversations();
  }, []);

  // Load messages when conversation is selected
  useEffect(() => {
    if (selectedConversationId) {
      loadMessages();
    }
  }, [selectedConversationId, messagesOffset]);

  const loadConversations = async () => {
    try {
      setConversationsLoading(true);
      setError(null);
      const response = await agentAPI.getMemory(100, 0);
      if (response.mode === 'conversations' && response.conversations) {
        setConversations(response.conversations);
      } else {
        setConversations([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load conversations');
    } finally {
      setConversationsLoading(false);
    }
  };

  const loadMessages = async () => {
    if (!selectedConversationId) return;

    try {
      setMessagesLoading(true);
      setError(null);
      const response = await agentAPI.getConversationMessages(
        selectedConversationId,
        messagesLimit,
        messagesOffset
      );
      if (response.mode === 'messages' && response.messages) {
        setMessages(response.messages);
        setMessagesTotal(response.total);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load messages');
    } finally {
      setMessagesLoading(false);
    }
  };

  const handleSelectConversation = (conversationId: string) => {
    setSelectedConversationId(conversationId);
    setMessagesOffset(0);
    setMessages([]);
  };

  const handleNextPage = () => {
    setMessagesOffset(prev => prev + messagesLimit);
  };

  const handlePrevPage = () => {
    setMessagesOffset(prev => Math.max(0, prev - messagesLimit));
  };

  const formatTimestamp = (timestamp: number) => {
    return new Date(timestamp * 1000).toLocaleString();
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
        return 'default' as const;
      case 'assistant':
        return 'secondary' as const;
      default:
        return 'outline' as const;
    }
  };

  const getCleanConversationId = (fullId: string) => {
    // Remove org prefix (e.g., "default-org:conv_123" -> "conv_123")
    const parts = fullId.split(':');
    if (parts.length > 1) {
      return parts.slice(1).join(':');
    }
    return fullId;
  };

  const filteredConversations = conversations.filter(conv =>
    !searchQuery ||
    conv.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (conv.last_message && conv.last_message.toLowerCase().includes(searchQuery.toLowerCase()))
  );

  const totalPages = Math.ceil(messagesTotal / messagesLimit);
  const currentPage = Math.floor(messagesOffset / messagesLimit) + 1;

  return (
    <div className="h-full flex">
      {/* Left Panel - Conversation List */}
      <div className="w-1/3 border-r border-border flex flex-col">
        {/* Left Panel Header */}
        <div className="p-4 border-b border-border">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              <h2 className="font-semibold">Conversations</h2>
              <Badge variant="secondary">{conversations.length}</Badge>
            </div>
            <Button variant="outline" size="sm" onClick={loadConversations} disabled={conversationsLoading}>
              <RefreshCw className={`h-4 w-4 ${conversationsLoading ? 'animate-spin' : ''}`} />
            </Button>
          </div>

          {/* Search */}
          <div className="relative">
            <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search conversations..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10"
            />
          </div>
        </div>

        {/* Conversation List */}
        <ScrollArea className="flex-1">
          <div className="p-2">
            {error && (
              <div className="mx-2 mb-4 p-3 bg-red-50 border border-red-200 rounded-lg">
                <p className="text-red-700 text-sm">{error}</p>
              </div>
            )}

            {conversationsLoading ? (
              <div className="flex items-center justify-center py-8">
                <div className="text-center">
                  <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 mx-auto mb-2"></div>
                  <p className="text-sm text-muted-foreground">Loading...</p>
                </div>
              </div>
            ) : filteredConversations.length === 0 ? (
              <div className="text-center py-8">
                <MessageSquare className="h-12 w-12 mx-auto mb-3 text-muted-foreground" />
                <p className="text-sm text-muted-foreground mb-1">No conversations found</p>
                <p className="text-xs text-muted-foreground">
                  {searchQuery ? 'Try adjusting your search' : 'Start chatting to see conversations'}
                </p>
              </div>
            ) : (
              <div className="space-y-2">
                {filteredConversations.map((conversation) => (
                  <Card
                    key={conversation.id}
                    className={`cursor-pointer transition-all hover:shadow-sm ${
                      selectedConversationId === conversation.id
                        ? 'ring-2 ring-blue-500 border-blue-200 bg-blue-50'
                        : 'hover:border-gray-300'
                    }`}
                    onClick={() => handleSelectConversation(conversation.id)}
                  >
                    <CardHeader className="pb-2">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <MessageSquare className="h-4 w-4 text-blue-500" />
                          <span className="font-medium text-sm">
                            {getCleanConversationId(conversation.id)}
                          </span>
                        </div>
                        <Badge variant="outline" className="text-xs">
                          {conversation.message_count}
                        </Badge>
                      </div>
                    </CardHeader>
                    <CardContent className="pt-0">
                      {conversation.last_message && (
                        <p className="text-xs text-muted-foreground mb-2 line-clamp-2">
                          {conversation.last_message.length > 100
                            ? conversation.last_message.substring(0, 100) + '...'
                            : conversation.last_message}
                        </p>
                      )}
                      <div className="flex items-center gap-1 text-xs text-muted-foreground">
                        <span>{formatTimestamp(conversation.last_activity)}</span>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </div>
        </ScrollArea>
      </div>

      {/* Right Panel - Message Detail */}
      <div className="flex-1 flex flex-col">
        {selectedConversationId ? (
          <>
            {/* Right Panel Header */}
            <div className="p-4 border-b border-border">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <MessageSquare className="h-5 w-5" />
                  <h2 className="font-semibold">
                    {getCleanConversationId(selectedConversationId)}
                  </h2>
                  <Badge variant="secondary">{messagesTotal} messages</Badge>
                </div>
                {totalPages > 1 && (
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-muted-foreground">
                      Page {currentPage} of {totalPages}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handlePrevPage}
                      disabled={currentPage === 1 || messagesLoading}
                    >
                      <ChevronLeft className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleNextPage}
                      disabled={currentPage === totalPages || messagesLoading}
                    >
                      <ChevronRight className="h-4 w-4" />
                    </Button>
                  </div>
                )}
              </div>
            </div>

            {/* Messages */}
            <ScrollArea className="flex-1">
              <div className="p-4">
                {messagesLoading && messages.length === 0 ? (
                  <div className="flex items-center justify-center py-8">
                    <div className="text-center">
                      <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-gray-900 mx-auto mb-3"></div>
                      <p className="text-sm">Loading messages...</p>
                    </div>
                  </div>
                ) : messages.length === 0 ? (
                  <div className="text-center py-12">
                    <MessageSquare className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
                    <p className="text-lg text-muted-foreground mb-2">No messages in this conversation</p>
                    <p className="text-sm text-muted-foreground">
                      Messages will appear here as the conversation develops
                    </p>
                  </div>
                ) : (
                  <div className="space-y-4">
                    {messages.map((message) => (
                      <Card key={message.id} className="hover:shadow-sm transition-shadow">
                        <CardHeader className="pb-3">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                              {getRoleIcon(message.role)}
                              <Badge variant={getRoleBadgeVariant(message.role)}>
                                {message.role}
                              </Badge>
                            </div>
                            <span className="text-xs text-muted-foreground">
                              {formatTimestamp(message.timestamp)}
                            </span>
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="prose prose-sm max-w-none">
                            <p className="whitespace-pre-wrap text-sm">{message.content}</p>
                          </div>
                          {message.metadata && Object.keys(message.metadata).length > 0 && (
                            <details className="mt-3">
                              <summary className="text-xs text-muted-foreground cursor-pointer">
                                Metadata
                              </summary>
                              <pre className="text-xs bg-muted p-2 rounded mt-1 overflow-auto">
                                {JSON.stringify(message.metadata, null, 2)}
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
          </>
        ) : (
          /* No conversation selected */
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <MessageSquare className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
              <p className="text-lg text-muted-foreground mb-2">Select a conversation</p>
              <p className="text-sm text-muted-foreground">
                Choose a conversation from the left panel to view its messages
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
