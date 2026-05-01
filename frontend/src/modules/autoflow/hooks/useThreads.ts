import { useState, useEffect, useCallback } from 'react';
import type { Thread, Message } from '../types';
import { getThreads, getMessages, sendMessage } from '../services/threadsService';

export function useThreads(orgId: string) {
  const [threads, setThreads] = useState<Thread[]>([]);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [isSending, setIsSending] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getThreads(orgId).then(data => {
      if (!cancelled) {
        setThreads(data);
        if (data.length > 0 && activeId === null) setActiveId(data[0].id);
      }
    });
    return () => { cancelled = true; };
  }, [orgId]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (activeId === null) return;
    let cancelled = false;
    getMessages(orgId, activeId).then(data => {
      if (!cancelled) {
        setMessages(data);
        window.dispatchEvent(new CustomEvent('autoflow:threads-updated'));
      }
    });
    return () => { cancelled = true; };
  }, [orgId, activeId]);

  const send = useCallback(async (content: string) => {
    if (activeId === null) return;
    setIsSending(true);
    try {
      const msg = await sendMessage(orgId, activeId, content);
      setMessages(prev => [...prev, msg]);
    } finally {
      setIsSending(false);
    }
  }, [orgId, activeId]);

  const activeThread = threads.find(t => t.id === activeId) ?? null;

  return { threads, activeThread, setActiveId, messages, send, isSending };
}
