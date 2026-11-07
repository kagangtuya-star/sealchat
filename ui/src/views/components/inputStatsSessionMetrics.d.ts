export interface InputStatMessage {
  createdAt: string | number | Date;
  charCount?: number;
}

export interface InputStatSession {
  index: number;
  startTime: string;
  endTime: string;
  duration: number;
  occupiedDuration: number;
  totalChars: number;
  totalMessages: number;
  typingSpeed: number;
}

export type InputStatSummary = Pick<InputStatSession, 'duration' | 'totalChars' | 'totalMessages'>;

export function buildInputStatSessions(messages: InputStatMessage[], thresholdMinutes: number): InputStatSession[];
export function calcSessionSummary(sessions: InputStatSession[]): InputStatSummary;
export function calcOccupiedWindowSummary(messages: InputStatMessage[], thresholdMinutes: number): InputStatSummary;
export function calcOccupiedWindowMinutes(messages: InputStatMessage[], thresholdMinutes: number): number;
