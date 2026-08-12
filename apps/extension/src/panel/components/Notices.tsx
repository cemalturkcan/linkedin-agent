interface NoticeProps {
  children: string
  action?: { label: string; run(): void }
}

function Notice({ children, action }: NoticeProps) {
  return (
    <div className="flex items-start gap-3 border-b border-hairline bg-raised px-3 py-2">
      <p className="min-w-0 flex-1 text-meta text-ink">{children}</p>
      {action ? (
        <button type="button" onClick={action.run} className="label shrink-0 text-muted hover:text-ink">
          {action.label}
        </button>
      ) : null}
    </div>
  )
}

interface NoticesProps {
  reachable: boolean
  unreachable: string | null
  paused: boolean
  stale: number
  notice: string | null
  onRetry(): void
  onResume(): void
  onRescreen(): void
  onDismiss(): void
}

export function Notices({
  reachable,
  unreachable,
  paused,
  stale,
  notice,
  onRetry,
  onResume,
  onRescreen,
  onDismiss,
}: NoticesProps) {
  return (
    <div className="shrink-0">
      {unreachable ? (
        <Notice action={{ label: 'retry', run: onRetry }}>{unreachable}</Notice>
      ) : null}
      {reachable && paused ? (
        <Notice action={{ label: 'resume', run: onResume }}>
          the agent is paused. nothing is planned, screened or attached until you switch it back on.
        </Notice>
      ) : null}
      {reachable && stale > 0 ? (
        <Notice action={{ label: 'rescreen', run: onRescreen }}>
          {`${stale} judged before your settings or cvs changed, so those verdicts may not hold.`}
        </Notice>
      ) : null}
      {notice ? <Notice action={{ label: 'dismiss', run: onDismiss }}>{notice}</Notice> : null}
    </div>
  )
}
