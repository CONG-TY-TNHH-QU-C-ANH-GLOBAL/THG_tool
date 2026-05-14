import PlatformShell from '@/src/platform/components/PlatformShell';

export default function PlatformLayout({ children }: { children: React.ReactNode }) {
  return <PlatformShell>{children}</PlatformShell>;
}
