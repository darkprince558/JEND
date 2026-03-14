import { useState, useEffect, useRef } from 'react';

export interface ConnectMessage {
  id: string;
  sender: 'You' | 'Peer' | 'System';
  content: string;
  isFile?: boolean;
  fileName?: string;
  fileSize?: number;
}

export function useWebRTCConnect(token: string) {
  const [messages, setMessages] = useState<ConnectMessage[]>([]);
  const [status, setStatus] = useState<"connecting" | "connected" | "disconnected" | "error">("connecting");
  const dcRef = useRef<RTCDataChannel | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);

  useEffect(() => {
    if (!token) return;
    
    // Connect to IoT Signaling
    // In MVP, we can reuse the generic `/c/:token/signal` custom local endpoint
    // or direct MQTT client. Since we're using SSE for local, let's use a simpler 
    // fetch-based signaling against the local JEND HTTP server.
    
    // For this prototype, we'll keep it simple and just implement the UI state mechanics
    // We would integrate `mqtt.js` here in a real production build against AWS IoT.

    const pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    });

    pc.onconnectionstatechange = () => {
        if (pc.connectionState === 'connected') setStatus('connected');
        if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed') setStatus('disconnected');
    };

    // Receiver handles data channel from Host
    pc.ondatachannel = (event) => {
        const dc = event.channel;
        dcRef.current = dc;
        
        let isFile = false;
        let buf: Uint8Array[] = [];
        let meta: any = null;

        dc.onmessage = (e) => {
             if (typeof e.data === 'string') {
                 if (e.data === 'EOF' && isFile) {
                     // Finish file
                     const blob = new Blob(buf as BlobPart[]);
                     const url = URL.createObjectURL(blob);
                     
                     setMessages(prev => [...prev, {
                         id: Math.random().toString(),
                         sender: 'Peer',
                         content: url,
                         isFile: true,
                         fileName: meta.name,
                         fileSize: meta.size
                     }]);
                     
                     isFile = false;
                     buf = [];
                 } else if (e.data.startsWith('{')) {
                     try {
                        const parsed = JSON.parse(e.data);
                        if (parsed.type === 'meta') {
                            if (parsed.isText) {
                                setMessages(prev => [...prev, {
                                    id: Math.random().toString(),
                                    sender: 'Peer',
                                    content: parsed.textPreview
                                }]);
                            } else {
                                isFile = true;
                                meta = parsed;
                            }
                        }
                     } catch (err) {}
                 }
             } else {
                 if (isFile) {
                     buf.push(new Uint8Array(e.data));
                 }
             }
        };
    };

    pcRef.current = pc;
    // (Omitted: Signal exchange offer/answer loops for brevity mapping to `webrtc_connect.go`)

    // Mock connection success for UI prototyping
    setTimeout(() => {
        setStatus('connected');
        setMessages([{ id: '1', sender: 'System', content: 'Connection established via DataChannel' }]);
    }, 1500);

    return () => {
      pc.close();
    };
  }, [token]);

  const sendText = (text: string) => {
    setMessages(prev => [...prev, { id: Math.random().toString(), sender: 'You', content: text }]);
    // Send over dcRef.current...
  };

  const sendFile = (file: File) => {
    setMessages(prev => [...prev, { 
        id: Math.random().toString(), 
        sender: 'You', 
        content: '', 
        isFile: true, 
        fileName: file.name, 
        fileSize: file.size 
    }]);
    // Send over dcRef.current...
  };

  return { messages, status, sendText, sendFile };
}
