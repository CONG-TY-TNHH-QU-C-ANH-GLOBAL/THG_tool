import React from 'react';
import { inputStyle } from '../../constants/styles';

type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (props, ref) => <input ref={ref} style={{ ...inputStyle, ...props.style }} {...props} />
);
Input.displayName = 'Input';
