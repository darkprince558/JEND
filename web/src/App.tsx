import type { ChangeEvent, DragEvent } from 'react';
import { useState } from 'react';
import { Upload, Download, ArrowRight, CheckCircle2, Loader2, Copy } from 'lucide-react';
import { useWebRTC } from './useWebRTC';

type Tab = 'send' | 'receive';

import { generateCode } from './words';

function App() {
  const [activeTab, setActiveTab] = useState<Tab>('send');

  // Transfer State
  const [transferCode, setTransferCode] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isReadyToSend, setIsReadyToSend] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [copiedTerminal, setCopiedTerminal] = useState(false);

  // Receiver Input State
  const [inputCode, setInputCode] = useState('');

  // WebRTC Hooks
  const { state: rtcState, connectSignaling, initWebRTC, startTransfer } = useWebRTC(
    activeTab === 'send' ? transferCode : inputCode,
    activeTab === 'send'
  );

  // -- SENDER ACTIONS --
  const handleFileSelect = (e: ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      setSelectedFile(file);
      const newCode = generateCode();
      setTransferCode(newCode);
      setIsReadyToSend(true);

      // Initialize Signaling immediately when file is picked
      connectSignaling(() => {
        // In full implementation, handle the SDP offers here
      });
      initWebRTC();
    }
  };

  const attemptTransfer = () => {
    if (selectedFile) startTransfer(selectedFile);
  };

  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(true);
  };

  const onDragLeave = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(false);
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      const file = e.dataTransfer.files[0];
      setSelectedFile(file);
      const newCode = generateCode();
      setTransferCode(newCode);
      setIsReadyToSend(true);
      connectSignaling(() => { });
      initWebRTC();
    }
  };

  const copyTerminalCommand = () => {
    navigator.clipboard.writeText(`./jend receive "${transferCode}"`);
    setCopiedTerminal(true);
    setTimeout(() => setCopiedTerminal(false), 2000);
  };

  // -- RECEIVER ACTIONS --
  const handleCodeInput = (val: string) => {
    // Basic sanitization
    setInputCode(val.toLowerCase().trim());
  };

  const handleReceiveConnect = () => {
    if (inputCode.length > 5) { // e.g. "a-b-c"
      connectSignaling(() => { });
      initWebRTC();
    }
  };

  return (
    <>
      <div className="bg-mesh" />

      <header style={{ padding: '2rem', display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
        <a href="/" style={{ textDecoration: 'none', color: 'inherit', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '1rem', cursor: 'pointer', transition: 'transform 0.2s', ...({ ':hover': { transform: 'scale(1.05)' } } as any) }}>
          <img src="./logo-arrow.png" alt="Arrow Logo" style={{ height: '48px', objectFit: 'contain' }} />
          <pre style={{
            margin: 0,
            fontSize: '10px',
            lineHeight: 1.2,
            fontFamily: 'monospace',
            color: 'hsl(var(--accent-base))',
            letterSpacing: '0'
          }}>{`     ██╗███████╗███╗   ██╗██████╗ 
     ██║██╔════╝████╗  ██║██╔══██╗
     ██║█████╗  ██╔██╗ ██║██║  ██║
██   ██║██╔══╝  ██║╚██╗██║██║  ██║
╚█████╔╝███████╗██║ ╚████║██████╔╝
 ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝`}</pre>
        </a>
      </header>

      <main>
        <div className="glass-panel animate-fade-in">

          {rtcState.status === 'idle' && (
            <div style={{
              display: 'flex',
              background: 'rgba(0,0,0,0.3)',
              borderRadius: '16px',
              padding: '4px',
              marginBottom: '2rem'
            }}>
              <button onClick={() => setActiveTab('send')} style={{
                flex: 1, padding: '0.75rem', borderRadius: '12px', border: 'none',
                background: activeTab === 'send' ? 'rgba(255,255,255,0.1)' : 'transparent',
                color: activeTab === 'send' ? 'white' : 'var(--text-secondary)',
                fontWeight: 600, cursor: 'pointer', transition: 'all 0.2s',
                display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.5rem'
              }}>
                <Upload size={18} /> Send File
              </button>
              <button onClick={() => setActiveTab('receive')} style={{
                flex: 1, padding: '0.75rem', borderRadius: '12px', border: 'none',
                background: activeTab === 'receive' ? 'rgba(255,255,255,0.1)' : 'transparent',
                color: activeTab === 'receive' ? 'white' : 'var(--text-secondary)',
                fontWeight: 600, cursor: 'pointer', transition: 'all 0.2s',
                display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.5rem'
              }}>
                <Download size={18} /> Receive
              </button>
            </div>
          )}

          <div style={{ textAlign: 'center' }}>

            {/* IN-PROGRESS UI */}
            {(rtcState.status === 'connecting' || rtcState.status === 'transferring') && (
              <div className="animate-fade-in">
                <Loader2 size={48} color="hsl(var(--accent-base))" style={{ animation: 'spin 2s linear infinite', margin: '0 auto 1rem auto' }} />
                <h2>{rtcState.status === 'connecting' ? 'Connecting to Peer...' : 'Transferring...'}</h2>

                {activeTab === 'send' && rtcState.status === 'connecting' && (
                  <div style={{ marginTop: '2rem', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>

                    {/* Web Request */}
                    <div style={{ padding: '1.5rem', background: 'rgba(0,0,0,0.3)', borderRadius: '16px', border: '1px solid var(--glass-border)' }}>
                      <p style={{ margin: 0, color: 'var(--text-secondary)' }}>Code for Web Receiver:</p>
                      <h1 style={{ letterSpacing: '2px', margin: '0.5rem 0 0 0', fontSize: '2.5rem' }}>{transferCode}</h1>
                    </div>

                    {/* Terminal Command */}
                    <div style={{ padding: '1.5rem', background: 'rgba(0,0,0,0.4)', borderRadius: '16px', border: '1px solid var(--glass-border)', display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                      <p style={{ margin: 0, color: 'var(--text-secondary)' }}>Send to terminal via CLI:</p>
                      <div style={{ display: 'flex', alignItems: 'center', background: 'hsl(var(--secondary))', padding: '0.75rem 1rem', borderRadius: '8px', cursor: 'pointer', transition: 'all 0.2s', border: '1px solid rgba(255,255,255,0.05)' }} onClick={copyTerminalCommand}>
                        <code style={{ flex: 1, textAlign: 'left', fontFamily: 'monospace', fontSize: '1.1rem', color: 'hsl(var(--accent-base))' }}>./jend receive "{transferCode}"</code>
                        <div style={{ background: copiedTerminal ? 'var(--success)' : 'rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '6px', display: 'flex', transition: 'all 0.2s' }}>
                          {copiedTerminal ? <CheckCircle2 size={16} color="#000" /> : <Copy size={16} color="var(--text-primary)" />}
                        </div>
                      </div>
                    </div>

                  </div>
                )}

                {rtcState.status === 'transferring' && (
                  <div style={{ marginTop: '2rem' }}>
                    <div style={{ height: '8px', background: 'rgba(255,255,255,0.1)', borderRadius: '4px', overflow: 'hidden' }}>
                      <div style={{ height: '100%', width: `${rtcState.progress}%`, background: 'hsl(var(--accent-base))', transition: 'width 0.2s' }} />
                    </div>
                    <p style={{ marginTop: '1rem', fontSize: '1.25rem', fontWeight: 'bold' }}>{rtcState.progress}%</p>
                  </div>
                )}
              </div>
            )}

            {/* DONE UI */}
            {rtcState.status === 'done' && (
              <div className="animate-fade-in">
                <div style={{
                  width: '80px', height: '80px', borderRadius: '50%',
                  background: 'rgba(45, 212, 191, 0.1)', display: 'flex', alignItems: 'center', justifyContent: 'center',
                  margin: '0 auto 1.5rem auto'
                }}>
                  <CheckCircle2 color="#2dd4bf" size={40} />
                </div>
                <h2>Transfer Complete!</h2>
                <p style={{ marginBottom: '2rem' }}>Connection successfully closed.</p>
                <button className="btn-primary" onClick={() => window.location.reload()}>New Transfer</button>
              </div>
            )}

            {/* INITIAL TABS */}
            {rtcState.status === 'idle' && activeTab === 'send' && (
              <div className="animate-fade-in" style={{ padding: '1rem 0' }}>
                <div style={{
                  width: '80px', height: '80px', borderRadius: '50%',
                  background: 'rgba(120, 0, 255, 0.1)', display: 'flex', alignItems: 'center', justifyContent: 'center',
                  margin: '0 auto 1.5rem auto'
                }}>
                  <Upload color="hsl(var(--accent-base))" size={32} />
                </div>
                <h2>Share a file securely</h2>

                {!isReadyToSend ? (
                  <div
                    onDragOver={onDragOver}
                    onDragLeave={onDragLeave}
                    onDrop={onDrop}
                    style={{
                      border: `2px dashed ${isDragging ? 'hsl(var(--accent-base))' : 'var(--glass-border)'}`,
                      borderRadius: '16px',
                      padding: '3rem 2rem',
                      background: isDragging ? 'rgba(120, 0, 255, 0.05)' : 'transparent',
                      transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                      transform: isDragging ? 'scale(1.02)' : 'scale(1)',
                      cursor: 'pointer'
                    }}
                    onClick={() => document.getElementById('file-upload')?.click()}
                  >
                    <Upload size={48} color={isDragging ? 'hsl(var(--accent-base))' : 'var(--text-secondary)'} style={{ margin: '0 auto 1.5rem auto', transition: 'all 0.3s' }} />
                    <h3 style={{ marginBottom: '0.5rem', fontSize: '1.25rem', fontWeight: 600 }}>Drag & Drop your file here</h3>
                    <p style={{ marginBottom: '1.5rem' }}>or click to browse from your device</p>
                    <label className="btn-primary" style={{ display: 'inline-block', width: 'auto', padding: '0.75rem 2rem' }} onClick={(e) => e.stopPropagation()}>
                      Select File
                      <input id="file-upload" type="file" style={{ display: 'none' }} onChange={handleFileSelect} />
                    </label>
                  </div>
                ) : (
                  <div className="animate-fade-in" style={{
                    background: 'rgba(0,0,0,0.2)', padding: '2rem', borderRadius: '16px', border: '1px solid var(--glass-border)'
                  }}>
                    <CheckCircle2 color="var(--success)" size={48} style={{ margin: '0 auto 1rem auto' }} />
                    <h3 style={{ marginBottom: '0.5rem', fontSize: '1.25rem' }}>File Selected</h3>
                    <p style={{ marginBottom: '2rem', color: 'var(--text-primary)', fontWeight: 500 }}>{selectedFile?.name}</p>
                    <button className="btn-primary" onClick={attemptTransfer}>Generate Transfer Code</button>
                  </div>
                )}
              </div>
            )}

            <div className="animate-fade-in" style={{ padding: '0.5rem 0' }}>
              <h2>Enter Transfer Code</h2>
              <p>Type the 3-word code to receive the file.</p>

              <div className="code-input-container" style={{ display: 'flex', justifyContent: 'center', margin: '2rem 0' }}>
                <input
                  className="code-char-input"
                  style={{ width: '100%', maxWidth: '300px', fontSize: '1.25rem' }}
                  placeholder="e.g. fast-happy-sloth"
                  value={inputCode}
                  onChange={(e) => handleCodeInput(e.target.value)}
                />
              </div>

              <button className="btn-primary" onClick={handleReceiveConnect} disabled={inputCode.length < 5} style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: '0.5rem', marginBottom: '2rem' }}>
                Connect <ArrowRight size={20} />
              </button>

              <div style={{ padding: '1.25rem', background: 'rgba(0,0,0,0.3)', borderRadius: '12px', border: '1px solid var(--glass-border)', fontSize: '0.9rem', color: 'var(--text-secondary)' }}>
                <strong style={{ color: 'var(--text-primary)' }}>Terminal User?</strong> Run <code style={{ color: 'hsl(var(--accent-base))', background: 'rgba(255,255,255,0.05)', padding: '0.2rem 0.4rem', borderRadius: '4px' }}>./jend send path/to/file</code> and type the code it generates here.
              </div>
            </div>

          </div>
        </div>
      </main>

      <footer style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.9rem' }}>
        <a href="https://github.com/darkprince558/jend" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--text-secondary)', textDecoration: 'none', transition: 'color 0.2s' }} onMouseEnter={(e) => e.currentTarget.style.color = 'var(--text-primary)'} onMouseLeave={(e) => e.currentTarget.style.color = 'var(--text-secondary)'}>
          Powered by JEND
        </a>
      </footer>
    </>
  );
}

export default App;
