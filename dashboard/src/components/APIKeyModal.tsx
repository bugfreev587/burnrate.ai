import { useState } from 'react'
import './APIKeyModal.css'

interface Props {
  apiKey: string
  onClose: () => void
}

export default function APIKeyModal({ apiKey, onClose }: Props) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(apiKey)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box" onClick={e => e.stopPropagation()}>
        <h2 className="modal-title">Your API Key</h2>
        <p className="modal-subtitle">
          Save this key now — it will <strong>not</strong> be shown again.
        </p>

        <div className="key-box">
          <code className="key-text">{apiKey}</code>
          <button className="copy-btn" onClick={handleCopy}>
            {copied ? 'Copied!' : 'Copy'}
          </button>
        </div>

        <div className="modal-usage">
          <p className="usage-title">Usage example:</p>
          <pre className="usage-code">{`export TOKENGATE_API_KEY="${apiKey}"`}</pre>
        </div>

        <button className="btn btn-primary modal-close-btn" onClick={onClose}>
          I've saved my key
        </button>
      </div>
    </div>
  )
}
