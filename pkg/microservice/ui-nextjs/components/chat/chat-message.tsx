'use client';

import React from 'react';
import type { ChatMessage } from '@/types/agent';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Copy, User, Bot } from 'lucide-react';

interface ChatMessageProps {
  message: ChatMessage;
}

export function ChatMessage({ message }: ChatMessageProps) {
  const formatTimestamp = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString();
  };

  const copyToClipboard = async () => {
    try {
      await navigator.clipboard.writeText(message.content);
    } catch (err) {
      console.error('Failed to copy text:', err);
    }
  };

  const isUser = message.role === 'user';
  const isSystem = message.role === 'system';

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div className={`flex space-x-3 max-w-[80%] ${isUser ? 'flex-row-reverse space-x-reverse' : ''}`}>
        {/* Avatar */}
        <Avatar className="h-8 w-8 mt-1">
          <AvatarFallback>
            {isUser ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
          </AvatarFallback>
        </Avatar>

        {/* Message Content */}
        <Card className={`${isUser ? 'bg-primary text-primary-foreground' : ''}`}>
          <CardContent className="p-3">
            {/* Header */}
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center space-x-2">
                <Badge
                  variant={isUser ? 'secondary' : isSystem ? 'outline' : 'default'}
                  className="text-xs"
                >
                  {message.role}
                </Badge>
                <span className="text-xs opacity-70">
                  {formatTimestamp(message.timestamp)}
                </span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 opacity-60 hover:opacity-100"
                onClick={copyToClipboard}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>

            {/* Content */}
            <div className="prose prose-sm max-w-none">
              <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed">
                {message.content}
              </pre>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}