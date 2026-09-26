import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import {App} from '@/App';
import {getSystem} from '@/api';
import '@/index.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// 浏览器标签页标题带上版本号（mock 模式跳过）
getSystem()
  .then((s) => {
    if (s.version && s.version !== 'mock') {
      document.title = `Tagger ${s.version} · 音乐档案工作台`;
    }
  })
  .catch(() => {});
