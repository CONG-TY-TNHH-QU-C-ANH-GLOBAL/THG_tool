import { useState, useEffect, useCallback } from 'react';
import type { Thread, Message } from '../types';
import { getThreads, getMessages, sendMessage } from '../services/threadsService';

export function useThreads(orgId: string) {
  const [threads, setThreads] = useState<Thread[]>([]);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [isSending, setIsSending] = useState(false);

  const fetchThreads = useCallback(async () => {
    const data = await getThreads(orgId);
    setThreads(data);
    setActiveId(current => {
      if (data.length === 0) return null;
      if (current !== null && data.some(thread => thread.id === current)) return current;
      return data[0].id;
    });
    return data;
  }, [orgId]);

  const fetchMessages = useCallback(async (threadId: number | null) => {
    if (threadId === null) {
      setMessages([]);
      return [];
    }
    const data = await getMessages(orgId, threadId);
    setMessages(data);
    window.dispatchEvent(new CustomEvent('autoflow:threads-updated'));
    return data;
  }, [orgId]);

  useEffect(() => {
    let cancelled = false;
    void getThreads(orgId).then(data => {
      if (cancelled) return;
      setThreads(data);
      setActiveId(current => {
        if (data.length === 0) return null;
        if (current !== null && data.some(thread => thread.id === current)) return current;
        return data[0].id;
      });
    });
    return () => { cancelled = true; };
  }, [orgId]);

  useEffect(() => {
    let cancelled = false;
    if (activeId === null) {
      setMessages([]);
      return () => { cancelled = true; };
    }
    void getMessages(orgId, activeId).then(data => {
      if (cancelled) return;
      setMessages(data);
      window.dispatchEvent(new CustomEvent('autoflow:threads-updated'));
    });
    return () => { cancelled = true; };
  }, [orgId, activeId]);

  const send = useCallback(async (content: string) => {
    if (activeId === null) return;
    setIsSending(true);
    try {
      const msg = await sendMessage(orgId, activeId, content);
      setMessages(prev => [...prev, msg]);
      window.dispatchEvent(new CustomEvent('autoflow:threads-updated'));
    } finally {
      setIsSending(false);
    }
  }, [orgId, activeId]);

  const refetch = useCallback(async () => {
    const nextThreads = await fetchThreads();
    const nextActiveId = activeId !== null && nextThreads.some(thread => thread.id === activeId)
      ? activeId
      : (nextThreads[0]?.id ?? null);
    await fetchMessages(nextActiveId);
  }, [activeId, fetchMessages, fetchThreads]);

  const activeThread = threads.find(thread => thread.id === activeId) ?? null;

  return { threads, activeThread, setActiveId, messages, send, isSending, refetch };
}
