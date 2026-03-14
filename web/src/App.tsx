import React, { useState, useEffect, useRef } from 'react';
import { 
  Upload, Camera, CheckboxOn, Cancel, Loader, Download, 
  FolderPlus, Lightbulb, Moon
} from 'pixelarticons/react';

const PixelIcon = ({ Icon, className = "" }: { Icon: any, className?: string }) => (
  <Icon className={className} shapeRendering="crispEdges" />
);

type AppMode = 'scan' | 'send' | 'receive' | 'connect';
type SendState = 'idle' | 'selected' | 'uploading' | 'success' | 'error';
type ReceiveState = 'incoming' | 'downloading' | 'success' | 'error';
type PayloadType = 'file' | 'text';
type Theme = 'light' | 'dark';

import { useWebRTCConnect } from './useWebRTCConnect';

interface FileInfo {
  name: string;
  size: number;
  type: string;
  hash: string;
}

function formatBytes(b: number) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(2) + ' GB';
}

export default function App() {
  const [mode, setMode] = useState<AppMode>('scan');
  const [sendState, setSendState] = useState<SendState>('idle');
  const [receiveState, setReceiveState] = useState<ReceiveState>('incoming');
  const [payloadType, setPayloadType] = useState<PayloadType>('file');
  const [progress, setProgress] = useState(0);
  const [textInput, setTextInput] = useState('');
  const [theme, setTheme] = useState<Theme>('dark');
  const [targetName, setTargetName] = useState('Laptop');
  const [appVersion, setAppVersion] = useState('v1.0');

  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [fileInfo, setFileInfo] = useState<FileInfo | null>(null);
  const [routeBase, setRouteBase] = useState('');
  const [errorMsg, setErrorMsg] = useState('');
  
  const fileInputRef = useRef<HTMLInputElement>(null);
  const cameraInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const path = window.location.pathname;
    if (path.startsWith('/u/')) {
      setMode('send');
      const parts = path.split('/');
      const base = `/${parts[1]}/${parts[2]}`;
      setRouteBase(base);
      
      fetch(`${base}/info`)
        .then(res => res.json())
        .then(data => {
          if (data.target) setTargetName(data.target);
          if (data.version) setAppVersion('v' + data.version.replace(/^v/, ''));
        })
        .catch(console.error);
        
    } else if (path.startsWith('/c/')) {
      setMode('connect');
      const parts = path.split('/');
      const base = `/${parts[1]}/${parts[2]}`;
      setRouteBase(base);
      
      fetch(`${base}/info`)
        .then(res => res.json())
        .then(data => {
          if (data.target) setTargetName(data.target);
          if (data.version) setAppVersion('v' + data.version.replace(/^v/, ''));
        })
        .catch(console.error);
        
    } else if (path.startsWith('/d/')) {
      setMode('receive');
      const parts = path.split('/');
      const base = `/${parts[1]}/${parts[2]}`;
      setRouteBase(base);
      
      fetch(`${base}/info`)
        .then(res => res.json())
        .then((data: FileInfo & { target?: string, version?: string }) => {
          setFileInfo(data);
          setPayloadType(data.type === 'text' ? 'text' : 'file');
          if (data.target) setTargetName(data.target);
          if (data.version) setAppVersion('v' + data.version.replace(/^v/, ''));
        })
        .catch(() => {
          setReceiveState('error');
          setErrorMsg('Failed to fetch file info. Check connection to laptop.');
        });
    }
  }, []);

  const connectToken = routeBase ? routeBase.split('/').pop() : '';
  const { messages, status: connectStatus, sendText, sendFile } = useWebRTCConnect(mode === 'connect' && connectToken ? connectToken : '');

  useEffect(() => {
    if (theme === 'dark') document.documentElement.classList.add('dark');
    else document.documentElement.classList.remove('dark');
  }, [theme]);

  const toggleTheme = () => setTheme(t => t === 'dark' ? 'light' : 'dark');

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      if (mode === 'connect') {
        sendFile(e.target.files[0]);
      } else {
        setSelectedFiles(Array.from(e.target.files));
        setSendState('selected');
        setErrorMsg('');
      }
    }
  };

  const handleDragOver = (e: React.DragEvent) => e.preventDefault();
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      setSelectedFiles(Array.from(e.dataTransfer.files));
      setSendState('selected');
      setErrorMsg('');
    }
  };

  const resetSend = () => {
    setSendState('idle');
    setSelectedFiles([]);
    setProgress(0);
    setErrorMsg('');
  };

  const resetReceive = () => {
    // If they click acknowledge, we can just reload the page or stay on success
    window.location.reload();
  };

  // UPLOAD
  const handleUpload = () => {
    if (selectedFiles.length === 0) return;
    setSendState('uploading');
    setProgress(0);

    const formData = new FormData();
    selectedFiles.forEach(file => formData.append('files', file));

    const xhr = new XMLHttpRequest();
    xhr.open('POST', `${routeBase}/upload`, true);
    
    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable) {
        setProgress(Math.round((e.loaded / e.total) * 100));
      }
    });

    xhr.onload = () => {
      if (xhr.status === 200) setSendState('success');
      else {
        setSendState('error');
        try {
          const res = JSON.parse(xhr.responseText);
          setErrorMsg(res.error || 'Upload failed');
        } catch {
          setErrorMsg('Upload failed');
        }
      }
    };

    xhr.onerror = () => {
      setSendState('error');
      setErrorMsg('Network error. Check WiFi connection.');
    };

    xhr.send(formData);
  };

  const handleSendText = () => {
    if (!textInput.trim()) return;
    setSendState('uploading');
    setProgress(100);

    fetch(`${routeBase}/upload-text`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: textInput })
    })
    .then(res => {
      if (res.ok) {
        setSendState('success');
        setTextInput('');
      } else throw new Error('Failed to send text');
    })
    .catch(err => {
      setSendState('error');
      setErrorMsg(err.message);
    });
  };

  // DOWNLOAD
  const handleDownload = async () => {
    if (!fileInfo) return;
    setReceiveState('downloading');
    setProgress(0);

    try {
      const response = await fetch(`${routeBase}/download`);
      if (!response.ok) throw new Error('HTTP ' + response.status);
      
      const total = parseInt(response.headers.get('content-length') || '0');
      const reader = response.body?.getReader();
      const chunks: Uint8Array[] = [];
      let received = 0;

      if (!reader) throw new Error('No reader available');

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value) {
          chunks.push(value);
          received += value.length;
          if (total > 0) setProgress(Math.round((received / total) * 100));
        }
      }

      const blob = new Blob(chunks as BlobPart[]);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = fileInfo.name || 'download';
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      
      setProgress(100);
      setReceiveState('success');
    } catch (err) {
      setReceiveState('error');
      setErrorMsg(err instanceof Error ? err.message : 'Download failed');
    }
  };

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-black text-zinc-900 dark:text-zinc-300 font-pixel flex flex-col items-center justify-center p-4 selection:bg-violet-500/30 transition-colors duration-200">
      <div className="text-center mb-6 flex flex-col items-center">
        <pre className="font-mono text-violet-600 dark:text-violet-500 text-[10px] sm:text-xs leading-none mb-3 text-left font-bold select-none">
{`     ██╗███████╗███╗   ██╗██████╗ 
     ██║██╔════╝████╗  ██║██╔══██╗
     ██║█████╗  ██╔██╗ ██║██║  ██║
██   ██║██╔══╝  ██║╚██╗██║██║  ██║
╚█████╔╝███████╗██║ ╚████║██████╔╝
 ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝`}
        </pre>
        <p className="text-zinc-500 text-sm uppercase tracking-widest">
          {mode === 'scan' && '> Invalid Environment'}
          {mode === 'send' && `> Target: ${targetName}`}
          {mode === 'receive' && `> Source: ${targetName}`}
          {mode === 'connect' && `> Peer: ${targetName}`}
        </p>
      </div>

      <div className="w-full max-w-[460px] bg-white dark:bg-[#0a0a0a] border border-zinc-300 dark:border-zinc-800 p-4 sm:p-5 relative transition-colors duration-200 shadow-sm dark:shadow-none">
        
        <div className="flex justify-between items-center mb-5">
          <div className="flex gap-4">
            {mode === 'connect' ? (
               <button className="text-xs uppercase tracking-widest pb-1 border-b-2 border-violet-500 text-zinc-900 dark:text-white transition-colors">
                 JEND CONNECT
               </button>
            ) : (
                <>
                <button 
                  disabled={mode !== 'send'}
                  className={`text-xs uppercase tracking-widest pb-1 border-b-2 transition-colors ${mode === 'send' ? 'border-violet-500 text-zinc-900 dark:text-white' : 'hidden'}`}
                >
                  Send to Laptop
                </button>
                <button 
                  disabled={mode !== 'receive'}
                  className={`text-xs uppercase tracking-widest pb-1 border-b-2 transition-colors ${mode === 'receive' ? 'border-emerald-500 text-zinc-900 dark:text-white' : 'hidden'}`}
                >
                  Receive from Laptop
                </button>
                {(mode === 'send' || mode === 'receive') && (
                  <a 
                    href="https://jend.app" target="_blank" rel="noopener noreferrer"
                    className="text-xs uppercase tracking-widest pb-1 border-b-2 border-transparent text-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors"
                    style={{ textDecoration: 'none' }}
                  >
                    Scan New QR
                  </a>
                )}
                </>
            )}
          </div>
          <button 
            onClick={toggleTheme}
            className="text-zinc-400 hover:text-zinc-900 dark:hover:text-white transition-colors"
            title="Toggle Theme"
          >
            {theme === 'dark' ? <PixelIcon Icon={Lightbulb} className="w-4 h-4" /> : <PixelIcon Icon={Moon} className="w-4 h-4" />}
          </button>
        </div>

        <div className="flex justify-between items-center mb-4 border-b border-zinc-300 dark:border-zinc-800 pb-3 transition-colors duration-200">
          <span className="text-xs text-zinc-500 uppercase tracking-widest">Status</span>
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 ${
              (sendState === 'error' || receiveState === 'error') ? 'bg-red-500 animate-pulse' :
              (sendState === 'success' || receiveState === 'success') ? 'bg-emerald-500' : 
              (sendState === 'uploading' || receiveState === 'downloading') ? 'bg-amber-500 animate-pulse' : 'bg-violet-500'
            }`}></div>
            <span className={`text-xs uppercase tracking-wider ${
              (sendState === 'error' || receiveState === 'error') ? 'text-red-500' :
              (sendState === 'success' || receiveState === 'success') ? 'text-emerald-600 dark:text-emerald-500' : 
              (sendState === 'uploading' || receiveState === 'downloading') ? 'text-amber-600 dark:text-amber-500' : 'text-violet-600 dark:text-violet-500'
            }`}>
              
              {mode === 'send' && sendState === 'idle' && 'Awaiting Input'}
              {mode === 'send' && sendState === 'selected' && 'Ready to Send'}
              {mode === 'send' && sendState === 'uploading' && 'Transmitting...'}
              {mode === 'send' && sendState === 'success' && 'Transfer Complete'}
              {mode === 'send' && sendState === 'error' && 'Transfer Failed'}
              
              {mode === 'receive' && receiveState === 'incoming' && 'Awaiting Accept'}
              {mode === 'receive' && receiveState === 'downloading' && 'Receiving...'}
              {mode === 'receive' && receiveState === 'success' && 'Transfer Complete'}
              {mode === 'receive' && receiveState === 'error' && 'Transfer Failed'}

              {mode === 'connect' && connectStatus === 'connecting' && 'Connecting...'}
              {mode === 'connect' && connectStatus === 'connected' && 'Session Active'}
              {mode === 'connect' && connectStatus === 'disconnected' && 'Disconnected'}
            </span>
          </div>
        </div>

        {mode === 'scan' && (
          <div className="text-center p-6 text-zinc-500 space-y-4">
             <p className="uppercase text-sm">Please launch this app by scanning a JEND QR code first.</p>
          </div>
        )}

        {/* SEND MODE */}
        {mode === 'send' && (
          <div className="space-y-4">
            {(sendState === 'idle' || sendState === 'error' || sendState === 'selected') && (
              <>
                <input 
                  type="file" 
                  multiple 
                  className="hidden" 
                  ref={fileInputRef} 
                  onChange={handleFileSelect} 
                />
                <input 
                  type="file" 
                  capture="environment" 
                  accept="image/*,video/*" 
                  className="hidden" 
                  ref={cameraInputRef} 
                  onChange={handleFileSelect} 
                />

                <div 
                  onClick={() => fileInputRef.current?.click()}
                  onDragOver={handleDragOver}
                  onDrop={handleDrop}
                  className={`border border-dashed p-6 flex flex-col items-center justify-center text-center transition-colors cursor-pointer ${
                    sendState === 'selected' 
                      ? 'border-violet-500 bg-violet-50 dark:bg-violet-500/5' 
                      : 'border-zinc-300 dark:border-zinc-700 hover:border-violet-500 hover:bg-zinc-50 dark:hover:bg-zinc-900'
                  }`}
                >
                  <PixelIcon Icon={FolderPlus} className={`w-6 h-6 mb-2 ${sendState === 'selected' ? 'text-violet-500 dark:text-violet-400' : 'text-zinc-400 dark:text-zinc-500'}`} />
                  <p className="text-sm text-zinc-800 dark:text-zinc-300 uppercase tracking-wider mb-1">Select Files</p>
                  <p className="text-xs text-zinc-500 dark:text-zinc-600">Drag & drop supported</p>
                </div>

                {sendState === 'error' && (
                  <p className="text-xs text-red-500 uppercase">{errorMsg}</p>
                )}

                {sendState === 'selected' ? (
                  <div className="space-y-3 pt-1">
                    <div className="space-y-2 max-h-40 overflow-y-auto screen-scroll">
                      {selectedFiles.map((f, i) => (
                        <div key={i} className="border border-zinc-300 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-3 flex items-center justify-between transition-colors duration-200">
                          <span className="text-sm text-zinc-800 dark:text-zinc-300 truncate">{f.name}</span>
                          <span className="text-xs text-zinc-500 shrink-0 ml-2">{formatBytes(f.size)}</span>
                        </div>
                      ))}
                    </div>

                    <div className="flex gap-2">
                       <button onClick={resetSend} className="px-4 py-2 border border-zinc-300 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-900 transition-colors uppercase tracking-widest text-xs flex items-center justify-center">
                         <PixelIcon Icon={Cancel} className="w-4 h-4" />
                       </button>
                       <button 
                         onClick={handleUpload}
                         className="flex-1 bg-violet-600 hover:bg-violet-700 dark:hover:bg-violet-500 text-white text-sm uppercase tracking-widest py-2.5 px-4 flex items-center justify-center transition-colors"
                       >
                         Execute Transfer
                       </button>
                    </div>
                  </div>
                ) : (
                  <div className="space-y-2 pt-1">
                    <div className="grid grid-cols-2 gap-3">
                      <button onClick={() => cameraInputRef.current?.click()} className="bg-zinc-50 dark:bg-zinc-900 hover:bg-zinc-100 dark:hover:bg-zinc-800 border border-zinc-300 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300 text-xs uppercase tracking-wider py-2 px-4 flex items-center justify-center gap-2 transition-colors">
                        <PixelIcon Icon={Camera} className="w-4 h-4" />
                        Camera
                      </button>
                      <button onClick={() => fileInputRef.current?.click()} className="bg-zinc-50 dark:bg-zinc-900 hover:bg-zinc-100 dark:hover:bg-zinc-800 border border-zinc-300 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300 text-xs uppercase tracking-wider py-2 px-4 flex items-center justify-center gap-2 transition-colors">
                        <PixelIcon Icon={Upload} className="w-4 h-4" />
                        Browse
                      </button>
                    </div>

                    <div className="relative pt-2">
                      <textarea 
                        value={textInput}
                        onChange={(e) => setTextInput(e.target.value)}
                        className="w-full block bg-zinc-50 dark:bg-zinc-950 border border-zinc-300 dark:border-zinc-800 p-3 pb-10 text-zinc-800 dark:text-zinc-300 placeholder-zinc-400 dark:placeholder-zinc-700 focus:outline-none focus:border-violet-500 transition-colors resize-none text-sm"
                        rows={2}
                        placeholder="> Enter text or URL..."
                      />
                      <button 
                        onClick={handleSendText}
                        className={`absolute bottom-3.5 right-3.5 px-3 py-1.5 text-xs uppercase tracking-wider transition-colors ${
                          textInput.trim() ? 'bg-violet-600 hover:bg-violet-700 dark:hover:bg-violet-500 text-white' : 'bg-zinc-200 dark:bg-zinc-900 text-zinc-400 dark:text-zinc-600 cursor-not-allowed'
                        }`}
                      >
                        Send Text
                      </button>
                    </div>
                  </div>
                )}
              </>
            )}

            {sendState === 'uploading' && (
              <div className="space-y-4 py-2">
                <div className="border border-zinc-300 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4 transition-colors duration-200">
                  <div className="flex justify-between text-xs text-zinc-500 dark:text-zinc-400 mb-2 uppercase tracking-wider">
                    <span>{selectedFiles.length > 0 ? selectedFiles[0].name : "Text Data"}</span>
                    <span>Uploading...</span>
                  </div>
                  <div className="h-2 w-full bg-zinc-200 dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-800 overflow-hidden">
                    <div 
                      className="h-full bg-amber-500 transition-all duration-200 ease-linear"
                      style={{ width: `${progress}%` }}
                    ></div>
                  </div>
                  <div className="mt-2 text-right text-xs text-amber-600 dark:text-amber-500">
                    {progress}%
                  </div>
                </div>
              </div>
            )}

            {sendState === 'success' && (
              <div className="space-y-4 py-2">
                <div className="border border-emerald-200 dark:border-emerald-900/50 bg-emerald-50 dark:bg-emerald-950/20 p-4 text-center transition-colors duration-200">
                  <PixelIcon Icon={CheckboxOn} className="w-8 h-8 text-emerald-500 mx-auto mb-2" />
                  <h3 className="text-emerald-600 dark:text-emerald-500 uppercase tracking-widest mb-1">Success</h3>
                  <p className="text-zinc-500 text-xs">Transmitted successfully.</p>
                </div>
                
                <button 
                  onClick={resetSend}
                  className="w-full bg-zinc-100 dark:bg-zinc-900 hover:bg-zinc-200 dark:hover:bg-zinc-800 border border-zinc-300 dark:border-zinc-800 text-zinc-800 dark:text-zinc-300 text-sm uppercase tracking-widest py-2.5 px-4 transition-colors"
                >
                  New Transfer
                </button>
              </div>
            )}
          </div>
        )}

        {/* RECEIVE MODE */}
        {mode === 'receive' && (
          <div className="space-y-4">
            {receiveState === 'error' && !fileInfo && (
               <div className="p-4 border border-red-500/30 bg-red-500/10 text-red-500 text-sm uppercase tracking-widest text-center">
                 {errorMsg}
               </div>
            )}
            {fileInfo && (
              <div className="border border-zinc-300 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-3 text-sm space-y-3 transition-colors duration-200">
                <div className="flex justify-between border-b border-zinc-200 dark:border-zinc-800/50 pb-2">
                  <span className="text-zinc-500 dark:text-zinc-600 uppercase tracking-widest text-xs">File</span>
                  <span className="text-zinc-800 dark:text-zinc-200 truncate pl-4">{fileInfo.name}</span>
                </div>
                <div className="flex justify-between border-b border-zinc-200 dark:border-zinc-800/50 pb-2">
                  <span className="text-zinc-500 dark:text-zinc-600 uppercase tracking-widest text-xs">Size</span>
                  <span className="text-zinc-800 dark:text-zinc-200">{formatBytes(fileInfo.size)}</span>
                </div>
                <div className="flex justify-between border-b border-zinc-200 dark:border-zinc-800/50 pb-2">
                  <span className="text-zinc-500 dark:text-zinc-600 uppercase tracking-widest text-xs">Type</span>
                  <span className="text-zinc-800 dark:text-zinc-200">{fileInfo.type}</span>
                </div>
                {fileInfo.hash && (
                  <div className="flex flex-col gap-1 pt-1">
                    <span className="text-zinc-500 dark:text-zinc-600 uppercase tracking-widest text-xs">SHA-256</span>
                    <span className="text-zinc-400 dark:text-zinc-500 text-xs break-all">{fileInfo.hash}</span>
                  </div>
                )}
              </div>
            )}

            <div className="pt-2">
              {receiveState === 'error' && fileInfo && (
                 <p className="text-xs text-red-500 uppercase text-center mb-4">{errorMsg}</p>
              )}
              {(receiveState === 'incoming' || (receiveState === 'error' && fileInfo)) && (
                <button 
                  onClick={handleDownload}
                  className="w-full bg-emerald-600 hover:bg-emerald-700 dark:hover:bg-emerald-500 text-white text-sm uppercase tracking-widest py-2.5 px-4 flex items-center justify-center gap-2 transition-colors"
                >
                  <PixelIcon Icon={Download} className="w-4 h-4" />
                  {payloadType === 'file' ? 'Accept File' : 'Accept Text'}
                </button>
              )}

              {receiveState === 'downloading' && (
                <div className="border border-zinc-300 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-3 transition-colors duration-200">
                  <div className="flex justify-between text-xs text-zinc-500 dark:text-zinc-400 mb-2 uppercase tracking-wider">
                    <span className="flex items-center gap-2"><PixelIcon Icon={Loader} className="w-4 h-4 animate-spin" /> Receiving</span>
                    <span>{progress}%</span>
                  </div>
                  <div className="h-2 w-full bg-zinc-200 dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-800 overflow-hidden">
                    <div 
                      className="h-full bg-amber-500 transition-all duration-200 ease-linear"
                      style={{ width: `${progress}%` }}
                    ></div>
                  </div>
                </div>
              )}

              {receiveState === 'success' && (
                <div className="space-y-3">
                  <button 
                    onClick={resetReceive}
                    className="w-full border border-emerald-500 text-emerald-600 dark:text-emerald-500 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 text-sm uppercase tracking-widest py-2.5 px-4 flex items-center justify-center gap-2 transition-colors"
                  >
                    <PixelIcon Icon={CheckboxOn} className="w-5 h-5" />
                    Complete
                  </button>
                </div>
              )}
            </div>
          </div>
        )}
        {/* CONNECT MODE */}
        {mode === 'connect' && (
          <div className="flex flex-col h-[60vh] sm:h-[400px]">
            <div className="flex-1 border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-[#111] overflow-y-auto p-4 space-y-3 mb-4 screen-scroll rounded-sm shadow-inner transition-colors">
              {messages.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center text-zinc-400 dark:text-zinc-600">
                  <span className="text-xs uppercase tracking-widest">Chat initialized</span>
                  <p className="text-[10px] mt-2 max-w-[200px] text-center opacity-70">
                    Send files and messages directly to your laptop over a secure WebRTC channel.
                  </p>
                </div>
              ) : (
                messages.map((msg, i) => (
                  <div key={i} className={`flex ${msg.sender === 'You' ? 'justify-end' : 'justify-start'}`}>
                    <div className={`max-w-[85%] text-sm px-3 py-2 ${
                      msg.sender === 'System'
                        ? 'bg-transparent text-zinc-500 text-xs italic text-center w-full mx-auto my-2 border-y border-zinc-200 dark:border-zinc-800/50 py-1'
                        : msg.sender === 'You'
                          ? 'bg-violet-600 text-white rounded-l-xl rounded-tr-xl rounded-br-sm shadow-sm'
                          : 'bg-white dark:bg-zinc-800 border-zinc-200 dark:border-zinc-700 border text-zinc-800 dark:text-zinc-200 rounded-r-xl rounded-tl-xl rounded-bl-sm shadow-sm'
                    }`}>
                      {msg.isFile ? (
                        <div className="flex items-center gap-2">
                          <PixelIcon Icon={Download} className="w-5 h-5 opacity-80" />
                          <div>
                            <p className="font-bold truncate max-w-[150px]">{msg.fileName}</p>
                            <p className="text-[10px] opacity-70">{formatBytes(msg.fileSize || 0)}</p>
                          </div>
                        </div>
                      ) : (
                        <p className="whitespace-pre-wrap break-words leading-relaxed">{msg.content}</p>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>

            <div className="flex gap-2 relative">
                <input
                  type="file"
                  className="hidden"
                  ref={fileInputRef}
                  onChange={handleFileSelect}
                />
                <button
                  onClick={() => fileInputRef.current?.click()}
                  className="bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-800 hover:bg-zinc-50 dark:hover:bg-zinc-800 p-3 text-zinc-600 dark:text-zinc-400 transition-colors shadow-sm rounded-sm"
                  title="Send File"
                >
                  <PixelIcon Icon={FolderPlus} className="w-4 h-4" />
                </button>

                <textarea
                  value={textInput}
                  onChange={(e) => setTextInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      if (textInput.trim()) {
                        sendText(textInput);
                        setTextInput('');
                      }
                    }
                  }}
                  placeholder="Message..."
                  rows={1}
                  className="flex-1 resize-none bg-white dark:bg-[#111] border border-zinc-300 dark:border-zinc-800 p-2 text-zinc-800 dark:text-zinc-200 focus:outline-none focus:border-violet-500 transition-colors shadow-sm rounded-sm text-sm"
                />

                <button
                  onClick={() => {
                    if (textInput.trim()) {
                      sendText(textInput);
                      setTextInput('');
                    }
                  }}
                  className="px-4 bg-violet-600 hover:bg-violet-700 text-white text-xs uppercase tracking-widest transition-colors shadow-sm rounded-sm disabled:opacity-50"
                  disabled={!textInput.trim() || connectStatus !== 'connected'}
                >
                  Send
                </button>
            </div>
          </div>
        )}

      </div>

      <div className="mt-6 text-center text-xs text-zinc-500 dark:text-zinc-600 uppercase tracking-widest">
        <a href="https://jend.app" target="_blank" rel="noopener noreferrer" className="hover:text-zinc-800 dark:hover:text-zinc-300 transition-colors">JEND Secure Protocol {appVersion}</a>
      </div>
    </div>
  );
}
