import { UserProfile } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'

export default function ProfilePage() {
  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content" style={{ display: 'flex', justifyContent: 'center' }}>
        <UserProfile routing="path" path="/profile" />
      </div>
    </div>
  )
}
