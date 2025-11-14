'use client';

import React from 'react';
import { ConversationInfo } from '@/types/agent';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { MessageSquare, Clock } from 'lucide-react';

interface ConversationListProps {
  conversations: ConversationInfo[];
  onSelectConversation: (conversationId: string) => void;
  selectedConversationId?: string | null;
}

export function ConversationList({
  conversations,
  onSelectConversation,
  selectedConversationId
}: ConversationListProps) {
  const formatTimestamp = (timestamp: number) => {
    return new Date(timestamp * 1000).toLocaleString();
  };

  if (conversations.length === 0) {
    return (
      <div className="text-center py-12">
        <MessageSquare className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
        <p className="text-lg text-muted-foreground mb-2">No conversations found</p>
        <p className="text-sm text-muted-foreground">
          Start chatting with the agent to see conversations here
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {conversations.map((conversation) => (
        <Card
          key={conversation.id}
          className={`cursor-pointer transition-all hover:shadow-md ${
            selectedConversationId === conversation.id
              ? 'ring-2 ring-blue-500 border-blue-200'
              : 'hover:border-gray-300'
          }`}
          onClick={() => onSelectConversation(conversation.id)}
        >
          <CardHeader className="pb-2">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <MessageSquare className="h-4 w-4 text-blue-500" />
                <span className="font-medium text-sm">
                  {conversation.id === 'default' ? 'Default Conversation' : `Conversation ${conversation.id}`}
                </span>
              </div>
              <Badge variant="outline" className="text-xs">
                {conversation.message_count} messages
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            {conversation.last_message && (
              <p className="text-sm text-muted-foreground mb-2 line-clamp-2">
                {conversation.last_message}
              </p>
            )}
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock className="h-3 w-3" />
              {formatTimestamp(conversation.last_activity)}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
