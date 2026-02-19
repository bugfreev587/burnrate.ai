import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'

const inter = Inter({ subsets: ['latin'] })

export const metadata: Metadata = {
  title: 'TokenGate — Take Control of Your AI',
  description:
    'The control layer between your AI tools and LLM providers. Visibility, guardrails, and budget caps for Claude Code, Cursor, and more.',
  openGraph: {
    title: 'TokenGate — Take Control of Your AI',
    description: 'Spend. Limits. Usage. Efficiency.',
    url: 'https://tokengate.to',
    siteName: 'TokenGate',
  },
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${inter.className} antialiased`}>{children}</body>
    </html>
  )
}
