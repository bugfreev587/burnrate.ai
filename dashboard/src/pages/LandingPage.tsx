import LandingNav from '../components/landing/LandingNav'
import LandingHero from '../components/landing/LandingHero'
import LandingSocialProof from '../components/landing/LandingSocialProof'
import LandingProblem from '../components/landing/LandingProblem'
import LandingSolution from '../components/landing/LandingSolution'
import LandingForSubscription from '../components/landing/LandingForSubscription'
import LandingForAPI from '../components/landing/LandingForAPI'
import LandingFeatures from '../components/landing/LandingFeatures'
import LandingHowItWorks from '../components/landing/LandingHowItWorks'
import LandingPricing from '../components/landing/LandingPricing'
import LandingFAQ from '../components/landing/LandingFAQ'
import LandingFinalCTA from '../components/landing/LandingFinalCTA'
import LandingFooter from '../components/landing/LandingFooter'
import { useEffect } from 'react'

export default function LandingPage() {
  useEffect(() => {
    const script = document.createElement('script')
    script.src = 'https://testimonial.to/js/iframeResizer.min.js'
    script.async = true
    script.onload = () => {
      if (typeof (window as any).iFrameResize === 'function') {
        (window as any).iFrameResize({ log: false, checkOrigin: false }, '#testimonialto-embed-text--OnQP-091OFCm4b2Afv7')
      }
    }
    document.body.appendChild(script)
    return () => { document.body.removeChild(script) }
  }, [])

  return (
    <main className="bg-[#06090f] text-slate-100">
      <LandingNav />
      <LandingHero />
      <LandingSocialProof />
      <LandingProblem />
      <iframe id="testimonialto-embed-text--OnQP-091OFCm4b2Afv7" src="https://embed-v2.testimonial.to/text/-OnQP-091OFCm4b2Afv7" frameBorder="0" scrolling="no" width="100%"></iframe>
      <LandingSolution />
      <LandingForSubscription />
      <LandingForAPI />
      <LandingFeatures />
      <LandingHowItWorks />
      <LandingPricing />
      <LandingFAQ />
      <LandingFinalCTA />
      <LandingFooter />
    </main>
  )
}
