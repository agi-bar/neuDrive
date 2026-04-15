import { useEffect, useMemo } from 'react'
import ReactCodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { EditorView } from '@codemirror/view'

type WorkbenchCodeEditorProps = {
  value: string
  isMarkdown: boolean
  onChange: (value: string) => void
  onReady?: (view: EditorView | null) => void
}

export default function WorkbenchCodeEditor({
  value,
  isMarkdown,
  onChange,
  onReady,
}: WorkbenchCodeEditorProps) {
  const extensions = useMemo(() => {
    const next = [EditorView.lineWrapping]
    if (isMarkdown) next.unshift(markdown())
    return next
  }, [isMarkdown])

  useEffect(() => {
    return () => onReady?.(null)
  }, [onReady])

  return (
    <div className="editor-codemirror-shell">
      <ReactCodeMirror
        value={value}
        height="100%"
        minHeight="100%"
        className="editor-codemirror"
        basicSetup={{
          foldGutter: false,
          highlightActiveLine: true,
          highlightActiveLineGutter: false,
          autocompletion: false,
          bracketMatching: true,
        }}
        extensions={extensions}
        onChange={onChange}
        onCreateEditor={(view) => onReady?.(view)}
      />
    </div>
  )
}
