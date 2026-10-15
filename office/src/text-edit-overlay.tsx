import { useRef, type ComponentProps } from 'react'
import { TextEditOverlay as OfficialTextEditOverlay } from '../vendor/slides/renderer/TextEditOverlay'

type Props = ComponentProps<typeof OfficialTextEditOverlay>

// An Agent can publish a new RenderTree during a manual edit. Keep the native
// overlay's initial node until that edit ends, so its DOM draft and selection
// survive the update. The bridge checks the original revision on commit.
export function TextEditOverlay(props: Props) {
  const initial = useRef(props.node)
  if (initial.current.id !== props.node.id) initial.current = props.node
  return <OfficialTextEditOverlay {...props} node={initial.current} />
}
