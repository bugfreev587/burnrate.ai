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
  return (
    <main className="bg-[#06090f] text-slate-100">
      <LandingNav />
      <LandingHero />
      <LandingSocialProof />
      <LandingProblem />
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
