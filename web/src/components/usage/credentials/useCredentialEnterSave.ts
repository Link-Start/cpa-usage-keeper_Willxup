import { useRef, type KeyboardEvent } from 'react'

// 两种凭证编辑入口共用：确认输入法候选词不能同时提交修改。
export function useCredentialEnterSave(save: () => Promise<void>) {
  const composing = useRef(false)
  return {
    onCompositionStart: () => { composing.current = true },
    onCompositionEnd: () => { composing.current = false },
    onBlur: () => { composing.current = false },
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (event.key !== 'Enter' || !(event.target instanceof HTMLInputElement) || event.target.type !== 'text') return
      event.preventDefault()
      // 部分浏览器确认候选词时已结束 composition，但仍以 229 标记该按键。
      if (composing.current || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return
      void save()
    },
  }
}
