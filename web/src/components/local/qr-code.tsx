// QRCode draws server-generated path data. It stays dark on light in both themes,
// because scanners need that contrast.
export function QRCode({ size, path, label }: { size: number; path: string; label: string }) {
  return (
    <svg
      className="qr-code"
      viewBox={`0 0 ${String(size)} ${String(size)}`}
      role="img"
      aria-label={label}
      shapeRendering="crispEdges"
    >
      <rect width={size} height={size} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  )
}
