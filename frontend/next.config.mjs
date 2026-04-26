/** @type {import('next').NextConfig} */
const nextConfig = {
  // Proxy all /api/* requests to the Go backend — avoids CORS in development.
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: `${process.env.API_URL ?? 'http://localhost:8080'}/api/:path*`,
      },
    ]
  },
}

export default nextConfig
