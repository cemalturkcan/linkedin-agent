export function departed<T extends { id: string }>(shown: T[], incoming: T[]): string[] {
  const staying = new Set(incoming.map((row) => row.id))
  return shown.filter((row) => !staying.has(row.id)).map((row) => row.id)
}
