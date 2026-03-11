import { useEffect } from 'react'
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

export default function LandingPage() {
  useEffect(() => {
    const script = document.createElement('script')
    script.src = 'https://testimonial.to/js/widget-embed.js'
    script.async = true
    document.body.appendChild(script)
    return () => { document.body.removeChild(script) }
  }, [])

  return (
    <main className="bg-[#06090f] text-slate-100">
      <LandingNav />
      <LandingHero />
      <LandingSocialProof />
      <LandingProblem />
      <iframe src="https://b4after.io/embed/kitchen-048204" width="100%" height="500" frameBorder="0"></iframe>
      <div className="testimonial-to-embed" data-url="https://embed-v2.testimonial.to/c/hello-world?theme=light" data-allow="camera;microphone" data-resize="true"></div>
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
