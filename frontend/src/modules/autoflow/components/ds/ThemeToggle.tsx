'use client';

import { useTheme } from 'next-themes';
import { Moon, Sun } from 'lucide-react';
import { useEffect, useState } from 'react';

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return <button className="btn btn-ghost btn-sm" style={{ width: 32, padding: 0 }} aria-label="Toggle theme" />;
  }

  const isDark = resolvedTheme === 'dark';

  return (
    <button
      type="button"
      className="btn btn-ghost btn-sm"
      style={{ width: 32, padding: 0, color: 'var(--text-faint)' }}
      onClick={() => setTheme(isDark ? 'light' : 'dark')}
      title={isDark ? 'Chuyển sang chế độ sáng' : 'Chuyển sang chế độ tối'}
      aria-label={isDark ? 'Chuyển sang chế độ sáng' : 'Chuyển sang chế độ tối'}
    >
      {isDark ? <Sun size={16} /> : <Moon size={16} />}
    </button>
  );
}
