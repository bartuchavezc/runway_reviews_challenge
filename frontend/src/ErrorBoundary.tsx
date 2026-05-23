import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: Error | null }

// Wraps <App> in main.tsx so a thrown render error doesn't blank the page.
// Errors here are by definition unexpected — there's no recovery path in
// app code, so the fallback offers a hard reload as the only action.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: { componentStack?: string | null }) {
    console.error('[ErrorBoundary] uncaught render error', error, info)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div style={fallbackStyle.wrap}>
        <h1 style={fallbackStyle.title}>Something went wrong</h1>
        <p style={fallbackStyle.desc}>
          The app hit an unexpected error and can't continue.
        </p>
        <button
          style={fallbackStyle.btn}
          onClick={() => window.location.reload()}
        >Reload</button>
      </div>
    )
  }
}

const fallbackStyle = {
  wrap: {
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
    gap: '14px',
    padding: '48px 32px',
    fontFamily: 'Geist, sans-serif',
    color: '#1a1a18',
    background: '#faf9f6',
  },
  title: { fontSize: '17px', fontWeight: 600, letterSpacing: '-0.3px', margin: 0 },
  desc: { fontSize: '13px', color: '#37362f', textAlign: 'center' as const, margin: 0 },
  btn: {
    marginTop: '6px',
    padding: '7px 20px',
    borderRadius: '7px',
    background: '#1a1a18',
    color: '#fff',
    fontSize: '13px',
    fontWeight: 500,
    border: 'none',
    cursor: 'pointer',
  },
}
