'use client';

import React, { useState, useEffect } from 'react';
import { AgentConfig } from '@/types/agent';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import { Settings, Moon, Sun, Zap } from 'lucide-react';

interface SettingsScreenProps {
  agentConfig: AgentConfig | null;
}

export function SettingsScreen({ agentConfig }: SettingsScreenProps) {
  const [theme, setTheme] = useState<'light' | 'dark'>('light');
  const [streamingEnabled, setStreamingEnabled] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(false);

  // Check if theme switching should be disabled (when ui_theme is set to "system")
  const isThemeSwitchDisabled = agentConfig?.ui_theme === 'system';

  useEffect(() => {
    // Only handle manual theme changes when not in system mode
    if (agentConfig?.ui_theme !== 'system') {
      if (theme === 'dark') {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    }

    // Update local state to reflect current theme (for display purposes)
    const isDarkMode = document.documentElement.classList.contains('dark');
    setTheme(isDarkMode ? 'dark' : 'light');
  }, [theme, agentConfig?.ui_theme]);

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="p-6 border-b border-border">
        <div className="flex items-center gap-2">
          <Settings className="h-6 w-6" />
          <h1 className="text-2xl font-bold">Settings</h1>
        </div>
      </div>

      <ScrollArea className="flex-1">
        <div className="p-6 space-y-6">
          {/* Appearance */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {theme === 'dark' ? <Moon className="h-5 w-5" /> : <Sun className="h-5 w-5" />}
                Appearance
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {isThemeSwitchDisabled ? (
                <div className="flex items-center justify-between">
                  <div>
                    <label className="text-sm font-medium">Theme</label>
                    <p className="text-xs text-muted-foreground">Following system theme preference</p>
                  </div>
                  <div className="flex items-center gap-2 text-muted-foreground">
                    {theme === 'dark' ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
                    <span className="text-sm">System</span>
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-between">
                  <div>
                    <label className="text-sm font-medium">Theme</label>
                    <p className="text-xs text-muted-foreground">Choose your preferred theme</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Sun className="h-4 w-4" />
                    <Switch
                      checked={theme === 'dark'}
                      onCheckedChange={(checked) => setTheme(checked ? 'dark' : 'light')}
                    />
                    <Moon className="h-4 w-4" />
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Chat Settings */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="h-5 w-5" />
                Chat Settings
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <label className="text-sm font-medium">Streaming Responses</label>
                  <p className="text-xs text-muted-foreground">
                    Enable real-time streaming of agent responses
                  </p>
                </div>
                <Switch
                  checked={streamingEnabled}
                  onCheckedChange={setStreamingEnabled}
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <label className="text-sm font-medium">Auto-refresh Data</label>
                  <p className="text-xs text-muted-foreground">
                    Automatically refresh agent data periodically
                  </p>
                </div>
                <Switch
                  checked={autoRefresh}
                  onCheckedChange={setAutoRefresh}
                />
              </div>
            </CardContent>
          </Card>



        </div>
      </ScrollArea>
    </div>
  );
}