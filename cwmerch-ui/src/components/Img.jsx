import { useEffect, useState } from "react";

// The image shown when a product/category has no image, or when its file
// can't be loaded (deleted upload, wrong URL...).
export const PLACEHOLDER_IMAGE = "/cwmerch-placeholder.svg";

// Img is an <img> that falls back to the default image.
const Img = ({ src, alt = "", className = "", ...rest }) => {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  const fallback = !src || failed;
  return (
    <img
      {...rest}
      src={fallback ? PLACEHOLDER_IMAGE : src}
      alt={alt}
      className={`${className} ${fallback ? "is-placeholder" : ""}`.trim() || undefined}
      onError={fallback ? undefined : () => setFailed(true)}
    />
  );
};

export default Img;
