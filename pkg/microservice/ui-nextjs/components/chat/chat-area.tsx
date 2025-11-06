'use client';

import React, { useState, useRef, useEffect } from 'react';
import { AgentConfig, ChatMessage } from '@/types/agent';
import { agentAPI } from '@/lib/api';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Send, Trash2, Loader2, RefreshCw, Building2 } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ChatMessage as ChatMessageComponent } from './chat-message';

interface ChatAreaProps {
  agentConfig: AgentConfig | null;
}

export function ChatArea({ agentConfig }: ChatAreaProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [conversationId, setConversationId] = useState<string>('');
  const [organizationId, setOrganizationId] = useState<string>('');
  const [charCount, setCharCount] = useState(0);
  const [streamingEnabled, setStreamingEnabled] = useState(true);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  useEffect(() => {
    setCharCount(input.length);
    adjustTextareaHeight();
  }, [input]);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  const adjustTextareaHeight = () => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      textarea.style.height = `${Math.min(textarea.scrollHeight, 120)}px`;
    }
  };

  const generateConversationId = () => {
    return `conv_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  };

  const generateNewConversationId = () => {
    // Use built-in crypto.randomUUID() for proper UUID v4 generation
    const newId = crypto.randomUUID();
    setConversationId(newId);
    // Clear messages when generating new conversation
    setMessages([]);
  };

  const addMessage = (message: ChatMessage) => {
    setMessages(prev => [...prev, message]);
  };

  const updateLastMessage = (content: string) => {
    setMessages(prev => {
      const newMessages = [...prev];
      if (newMessages.length > 0 && newMessages[newMessages.length - 1].role === 'assistant') {
        newMessages[newMessages.length - 1] = {
          ...newMessages[newMessages.length - 1],
          content: content,
        };
      }
      return newMessages;
    });
  };

  const sendMessage = async () => {
    if (!input.trim() || isLoading) return;

    const userMessage: ChatMessage = {
      role: 'user',
      content: input.trim(),
      timestamp: Date.now(),
      id: `msg_${Date.now()}_user`,
    };

    addMessage(userMessage);
    setInput('');
    setIsLoading(true);

    // Generate conversation ID if not exists
    const currentConversationId = conversationId || generateConversationId();
    if (!conversationId) {
      setConversationId(currentConversationId);
    }

    try {
      if (streamingEnabled) {
        // Streaming response
        const assistantMessage: ChatMessage = {
          role: 'assistant',
          content: '',
          timestamp: Date.now(),
          id: `msg_${Date.now()}_assistant`,
        };
        addMessage(assistantMessage);

        const stream = agentAPI.streamAgent({
          input: userMessage.content,
          conversation_id: currentConversationId,
          org_id: organizationId || undefined,
        });

        let fullContent = '';
        for await (const eventData of stream) {
          if (eventData.error) {
            throw new Error(eventData.error);
          }

          if (eventData.type === 'content' && eventData.content) {
            fullContent += eventData.content;
            updateLastMessage(fullContent);
          }

          if (eventData.is_final || eventData.type === 'done') {
            break;
          }
        }
      } else {
        // Non-streaming response
        const response = await agentAPI.runAgent({
          input: userMessage.content,
          conversation_id: currentConversationId,
          org_id: organizationId || undefined,
        });

        const assistantMessage: ChatMessage = {
          role: 'assistant',
          content: response.output,
          timestamp: Date.now(),
          id: `msg_${Date.now()}_assistant`,
        };
        addMessage(assistantMessage);
      }
    } catch (error) {
      console.error('Error sending message:', error);
      const errorMessage: ChatMessage = {
        role: 'assistant',
        content: `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`,
        timestamp: Date.now(),
        id: `msg_${Date.now()}_error`,
      };
      addMessage(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  const clearChat = () => {
    setMessages([]);
    setConversationId('');
    // Don't clear organization ID as user might want to keep it
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Messages Area */}
      <div className="flex-1 overflow-hidden">
        <ScrollArea className="h-full">
          <div className="p-4">
            {messages.length === 0 ? (
              <div className="flex items-center justify-center min-h-[400px]">
                <Card className="p-8 text-center max-w-md">
                  <h2 className="text-2xl font-semibold mb-4">Welcome to Agent UI</h2>
                  <p className="text-muted-foreground mb-4">
                    Start a conversation by typing a message below.
                  </p>
                  {agentConfig && (
                    <div className="space-y-2">
                      <Badge variant="outline">Agent: {agentConfig.name}</Badge>
                      <Badge variant="outline">Model: {agentConfig.model}</Badge>
                    </div>
                  )}
                </Card>
              </div>
            ) : (
              <div className="space-y-4">
                {messages.map((message) => (
                  <ChatMessageComponent key={message.id} message={message} />
                ))}
                {isLoading && (
                  <div className="flex items-center space-x-2 text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    <span>Agent is thinking...</span>
                  </div>
                )}
                <div ref={messagesEndRef} />
              </div>
            )}
          </div>
        </ScrollArea>
      </div>

      {/* Input Area */}
      <div className="border-t border-border p-4">
        <div className="flex space-x-2">
          <div className="flex-1">
            <Textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type your message here..."
              className="min-h-[60px] max-h-[120px] resize-none"
              disabled={isLoading}
            />
            <div className="flex items-center justify-between mt-2 gap-4">
              <div className="flex items-center space-x-3 flex-1">
                <div className="flex items-center space-x-2">
                  <Switch
                    id="streaming-mode"
                    checked={streamingEnabled}
                    onCheckedChange={setStreamingEnabled}
                    disabled={isLoading}
                  />
                  <label
                    htmlFor="streaming-mode"
                    className="text-xs font-medium cursor-pointer"
                  >
                    Streaming
                  </label>
                </div>

                <div className="flex items-center space-x-2">
                  <Building2 className="h-3 w-3 text-muted-foreground" />
                  <Label htmlFor="org-id" className="text-xs">Org:</Label>
                  <Input
                    id="org-id"
                    type="text"
                    value={organizationId}
                    onChange={(e) => setOrganizationId(e.target.value)}
                    placeholder="default"
                    className="h-7 w-24 text-xs"
                    disabled={isLoading}
                  />
                </div>

                <div className="flex items-center space-x-2">
                  <Label htmlFor="conv-id" className="text-xs">Conv:</Label>
                  <Input
                    id="conv-id"
                    type="text"
                    value={conversationId}
                    onChange={(e) => setConversationId(e.target.value)}
                    placeholder="auto-generate"
                    className="h-7 w-32 text-xs"
                    disabled={isLoading}
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={generateNewConversationId}
                    disabled={isLoading}
                    className="h-7 w-7 p-0"
                    title="Generate new UUID"
                  >
                    <RefreshCw className="h-3 w-3" />
                  </Button>
                </div>

                <span className="text-xs text-muted-foreground ml-2">
                  {charCount} chars
                </span>
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={clearChat}
                  disabled={messages.length === 0}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
                <Button
                  onClick={sendMessage}
                  disabled={!input.trim() || isLoading}
                  size="sm"
                >
                  {isLoading ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Send className="h-4 w-4" />
                  )}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}