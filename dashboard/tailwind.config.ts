import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './src/components/landing/**/*.{ts,tsx}',
    './src/pages/LandingPage.tsx',
  ],
  corePlugins: {
    // Disable Preflight so it doesn't reset the existing dashboard CSS
    preflight: false,
  },
  theme: {
    extend: {},
  },
  plugins: [],
}

export default config
