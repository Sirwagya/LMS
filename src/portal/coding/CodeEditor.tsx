import { useEffect, useRef } from "react";
import { EditorView, basicSetup } from "codemirror";
import { oneDark } from "@codemirror/theme-one-dark";
import { keymap } from "@codemirror/view";
import { indentWithTab } from "@codemirror/commands";
import type { Language } from "./types";
const languageSupport = {
  python: () => import("@codemirror/lang-python").then((m) => m.python()),
  java: () => import("@codemirror/lang-java").then((m) => m.java()),
};
export function CodeEditor({
  source,
  language,
  onChange,
}: {
  source: string;
  language: Language;
  onChange: (source: string) => void;
}) {
  const host = useRef<HTMLDivElement>(null),
    view = useRef<EditorView | null>(null),
    latest = useRef(source),
    change = useRef(onChange);
  useEffect(() => {
    change.current = onChange;
  }, [onChange]);
  useEffect(() => {
    latest.current = source;
    const editor = view.current;
    if (editor && editor.state.doc.toString() !== source)
      editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: source },
      });
  }, [source]);
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let active = true;
    let editor: EditorView | null = null;
    void languageSupport[language]().then((support) => {
      if (!active) return;
      editor = new EditorView({
        parent: element,
        doc: latest.current,
        extensions: [
          basicSetup,
          support,
          oneDark,
          keymap.of([indentWithTab]),
          EditorView.contentAttributes.of({
            "aria-label": `${language === "java" ? "Java" : "Python"} code editor`,
          }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) change.current(u.state.doc.toString());
          }),
          EditorView.theme({
            "&": { fontSize: "13px", height: "390px" },
            ".cm-scroller": {
              fontFamily: '"JetBrains Mono",ui-monospace,monospace',
              overflow: "auto",
            },
            ".cm-content": { padding: "18px 0" },
            ".cm-gutters": { paddingLeft: "8px" },
          }),
        ],
      });
      view.current = editor;
    });
    return () => {
      active = false;
      view.current = null;
      editor?.destroy();
    };
  }, [language]);
  return <div className="coding-editor" ref={host} />;
}
