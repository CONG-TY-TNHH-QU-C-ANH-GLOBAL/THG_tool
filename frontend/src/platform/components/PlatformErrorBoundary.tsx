'use client';
import { Component, type ReactNode } from 'react';

interface State { hasError: boolean; message?: string }

export class PlatformErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(err: unknown): State {
    return { hasError: true, message: err instanceof Error ? err.message : String(err) };
  }

  componentDidCatch(err: unknown) {
    if (process.env.NODE_ENV !== 'production') {
      console.error('[platform] error boundary caught', err);
    }
  }

  reset = () => this.setState({ hasError: false, message: undefined });

  render() {
    if (this.state.hasError) {
      return (
        <div style={{ display: 'grid', placeItems: 'center', padding: 40, minHeight: '60vh' }}>
          <div className="card" style={{ maxWidth: 480, padding: 28, textAlign: 'center' }}>
            <h2 style={{ fontSize: 18, marginBottom: 8 }}>Đã có lỗi xảy ra</h2>
            <p style={{ color: 'var(--text-mute)', fontSize: 13, marginBottom: 16 }}>{this.state.message || 'Unexpected error'}</p>
            <button className="btn btn-primary btn-sm" type="button" onClick={this.reset}>Thử lại</button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
