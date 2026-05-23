// Display-layer helpers that derive an app's swatch tint and 2-letter
// initials from the backend-supplied id and name. Lives on the frontend
// because neither value is meaningful data — they're presentation choices
// (color palette is invented; initials are a UI affordance, not a fact
// about the app) and have no place in the persistence struct.

// Pastel palette — designed to carry dark text legibly at all swatch sizes
// (16×16 sidebar, 22×22 settings row, 40×40 app header). Saturated brand
// colours were unreadable with the dark text token because rgba-blended
// foregrounds drop below WCAG-AA contrast on a dark backdrop.
const TINT_PALETTE = [
  '#c9d2f2', // soft indigo
  '#f9d0bd', // peach
  '#c4e0d3', // sage
  '#dcc6e6', // lilac
  '#f5dcc1', // warm tan
  '#c8dcea', // sky
  '#eccac3', // coral
  '#c0dcd2', // mint
]

// Deterministic palette pick from the app id. Uses a tiny string hash so
// the same id always lands on the same colour across reloads, and so two
// independently-added apps don't pile up on tint #0.
export function tint(id: string): string {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = ((hash << 5) - hash + id.charCodeAt(i)) | 0
  }
  return TINT_PALETTE[Math.abs(hash) % TINT_PALETTE.length]
}

// Short label for a windowHours value (e.g. 168 → "1W"). Matches the
// FilterBar chip's short label so the header stat tile stays consistent
// with the chip the user just toggled. Uppercased via CSS at the call site.
export function windowLabel(hours: number): string {
  switch (hours) {
    case 0:    return 'All'
    case 24:   return '24h'
    case 48:   return '48h'
    case 72:   return '3d'
    case 168:  return '1W'
    case 720:  return '1M'
    case 8760: return '1Y'
    default:   return `${hours}h`
  }
}

// 2-letter badge. Drops punctuation / dashes ("Notion – Notes, Docs" → NN)
// and falls back to repeating the first character for single-letter names.
export function initials(name: string): string {
  if (!name) return '??'
  const words = name
    .split(/[\s\-_–—]+/)
    .filter(w => /^[A-Za-z]/.test(w))
  if (words.length === 0) {
    const c = name[0].toUpperCase()
    return c + c
  }
  if (words.length === 1) {
    const w = words[0]
    return (w[0] + (w[1] ?? w[0])).toUpperCase()
  }
  return (words[0][0] + words[1][0]).toUpperCase()
}
