export default function Logo({ size = 32, withText = false }) {
  return (
    <span className="logo">
      <img src="/logo.svg" alt="PostDream" style={{ height: size, width: 'auto' }} />
      {withText && (
        <span className="logo-text">
          Post<b>Dream</b>
        </span>
      )}
    </span>
  )
}
