/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',

  async redirects() {
    const legacyRoutes = [
      '/dashboard', '/leads', '/accounts', '/browser',
      '/jobs', '/sessions', '/learning',
    ]
    return legacyRoutes.map(source => ({
      source,
      destination: '/',
      permanent: false, // 307 — không cache vĩnh viễn, dễ thay đổi sau
    }))
  },

  async rewrites() {
    return [
      {
        source: '/join/:token*',
        destination: '/',
      },
      {
        source: '/api/:path*',
        destination: `${process.env.API_URL ?? 'http://localhost:8080'}/api/:path*`,
      },
    ]
  },
}

export default nextConfig
