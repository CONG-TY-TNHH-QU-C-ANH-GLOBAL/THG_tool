/** @type {import('next').NextConfig} */
const nextConfig = {
  // Standalone output bundles only runtime node_modules — deploy one directory.
  output: 'standalone',

  // Proxy /api/* to the Go API server in dev (port 8081).
  // In production, nginx routes /api/v1/* directly to cmd/api — no extra hop.
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: `${process.env.API_URL ?? 'http://localhost:8081'}/api/:path*`,
      },
    ]
  },
}

export default nextConfig
