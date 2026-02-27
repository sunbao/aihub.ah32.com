import { useEffect, useMemo, useRef } from "react";
import * as THREE from "three";

export type SquarePlanetNode = {
  id: string;
  label: string;
  runId: string;
};

function clamp(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

function seedFromString(s: string): number {
  const str = String(s ?? "");
  let h = 2166136261;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function fibonacciSphere(i: number, n: number): THREE.Vector3 {
  const y = 1 - (i / (n - 1)) * 2; // 1..-1
  const radius = Math.sqrt(1 - y * y);
  const phi = Math.PI * (3 - Math.sqrt(5)) * i;
  return new THREE.Vector3(Math.cos(phi) * radius, y, Math.sin(phi) * radius);
}

function roundRectPath(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  const rr = Math.max(0, Math.min(r, Math.min(w, h) / 2));
  ctx.moveTo(x + rr, y);
  ctx.arcTo(x + w, y, x + w, y + h, rr);
  ctx.arcTo(x + w, y + h, x, y + h, rr);
  ctx.arcTo(x, y + h, x, y, rr);
  ctx.arcTo(x, y, x + w, y, rr);
  ctx.closePath();
}

function isCanvasLabelSupported(): boolean {
  try {
    const c = document.createElement("canvas");
    return Boolean(c.getContext("2d"));
  } catch {
    return false;
  }
}

export function SquarePlanetThree({
  nodes,
  onSelect,
  className,
}: {
  nodes: SquarePlanetNode[];
  onSelect: (node: SquarePlanetNode) => void;
  className?: string;
}) {
  const rootRef = useRef<HTMLDivElement | null>(null);

  const prepared = useMemo(() => {
    const list = nodes.slice(0, 60);
    const fillCount = 120;
    const allCount = list.length + fillCount;

    const fill = new Array(fillCount).fill(0).map((_, i) => {
      const p = fibonacciSphere(i, fillCount);
      return { pos: p, hue: 205 + (i % 60) };
    });

    const clickable = list.map((n, idx) => {
      const rnd = mulberry32(seedFromString(n.id));
      // slight randomization around a fibonacci distribution
      const p = fibonacciSphere(idx, Math.max(12, list.length));
      p.x += (rnd() - 0.5) * 0.12;
      p.y += (rnd() - 0.5) * 0.12;
      p.z += (rnd() - 0.5) * 0.12;
      p.normalize();
      const hue = Math.floor(190 + rnd() * 110);
      return { node: n, pos: p, hue };
    });

    return { clickable, fill, allCount };
  }, [nodes]);

  useEffect(() => {
    const mount = rootRef.current;
    if (!mount) return;
    const mountEl = mount;

    let disposed = false;
    const canLabel = isCanvasLabelSupported();

    const prefersReducedMotion = (() => {
      try {
        return window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
      } catch {
        return false;
      }
    })();

    const scene = new THREE.Scene();

    const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 100);
    camera.position.set(0, 0.15, 4.1);

    const renderer = new THREE.WebGLRenderer({
      antialias: true,
      alpha: true,
      powerPreference: "high-performance",
    });
    renderer.setClearColor(0x000000, 0);

    const overlay = document.createElement("canvas");
    overlay.style.position = "absolute";
    overlay.style.inset = "0";
    overlay.style.pointerEvents = "none";

    const overlayCtx = canLabel ? overlay.getContext("2d") : null;

    const container = document.createElement("div");
    container.style.position = "relative";
    container.style.width = "100%";
    container.style.height = "100%";
    container.appendChild(renderer.domElement);
    container.appendChild(overlay);
    mountEl.appendChild(container);

    const root = new THREE.Group();
    scene.add(root);

    // Lights
    const ambient = new THREE.AmbientLight(0xffffff, 0.55);
    scene.add(ambient);
    const key = new THREE.DirectionalLight(0xffffff, 0.95);
    key.position.set(2.3, 2.1, 2.6);
    scene.add(key);
    const rim = new THREE.DirectionalLight(0x88ddff, 0.65);
    rim.position.set(-2.4, 0.3, -2.0);
    scene.add(rim);

    // Planet body
    const planetRadius = 1.0;
    const geo = new THREE.SphereGeometry(planetRadius, 64, 48);
    const mat = new THREE.MeshPhysicalMaterial({
      color: new THREE.Color("#9b84ff"),
      roughness: 0.18,
      metalness: 0.08,
      transmission: 0.86,
      thickness: 0.7,
      ior: 1.28,
      clearcoat: 0.55,
      clearcoatRoughness: 0.2,
      transparent: true,
      opacity: 0.96,
    });
    const planet = new THREE.Mesh(geo, mat);
    root.add(planet);

    // Rim glow (fresnel-ish)
    const rimGeo = new THREE.SphereGeometry(planetRadius * 1.03, 64, 48);
    const rimMat = new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      uniforms: {
        uColorA: { value: new THREE.Color("#66e3ff") },
        uColorB: { value: new THREE.Color("#c18bff") },
      },
      vertexShader: `
        varying vec3 vN;
        varying vec3 vV;
        void main() {
          vN = normalize(normalMatrix * normal);
          vec4 mv = modelViewMatrix * vec4(position, 1.0);
          vV = normalize(-mv.xyz);
          gl_Position = projectionMatrix * mv;
        }
      `,
      fragmentShader: `
        uniform vec3 uColorA;
        uniform vec3 uColorB;
        varying vec3 vN;
        varying vec3 vV;
        void main() {
          float f = 1.0 - max(dot(vN, vV), 0.0);
          float a = pow(f, 2.4) * 0.55;
          vec3 c = mix(uColorA, uColorB, clamp(vN.y*0.5+0.5, 0.0, 1.0));
          gl_FragColor = vec4(c, a);
        }
      `,
    });
    const rimMesh = new THREE.Mesh(rimGeo, rimMat);
    root.add(rimMesh);

    // Ring
    const ringGeo = new THREE.TorusGeometry(planetRadius * 1.35, 0.06, 24, 220);
    const ringMat = new THREE.MeshStandardMaterial({
      color: new THREE.Color("#bfa7ff"),
      roughness: 0.55,
      metalness: 0.18,
      transparent: true,
      opacity: 0.55,
      emissive: new THREE.Color("#3dbcf2"),
      emissiveIntensity: 0.12,
    });
    const ring = new THREE.Mesh(ringGeo, ringMat);
    ring.rotation.x = Math.PI / 2.7;
    ring.rotation.y = Math.PI / 6;
    root.add(ring);

    // Starfield
    const starsGeo = new THREE.BufferGeometry();
    const starCount = 420;
    const starPos = new Float32Array(starCount * 3);
    const starSize = new Float32Array(starCount);
    const sr = mulberry32(0x0badcafe);
    for (let i = 0; i < starCount; i++) {
      const r = 7.0 * Math.pow(sr(), 0.65);
      const theta = sr() * Math.PI * 2;
      const phi = Math.acos(2 * sr() - 1);
      const sinPhi = Math.sin(phi);
      starPos[i * 3 + 0] = r * sinPhi * Math.cos(theta);
      starPos[i * 3 + 1] = r * Math.cos(phi);
      starPos[i * 3 + 2] = r * sinPhi * Math.sin(theta);
      starSize[i] = lerp(0.6, 1.6, sr());
    }
    starsGeo.setAttribute("position", new THREE.BufferAttribute(starPos, 3));
    starsGeo.setAttribute("size", new THREE.BufferAttribute(starSize, 1));
    const starsMat = new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      uniforms: {
        uOpacity: { value: 0.28 },
      },
      vertexShader: `
        attribute float size;
        varying float vSize;
        void main() {
          vSize = size;
          vec4 mv = modelViewMatrix * vec4(position, 1.0);
          gl_Position = projectionMatrix * mv;
          gl_PointSize = size * (300.0 / -mv.z);
        }
      `,
      fragmentShader: `
        uniform float uOpacity;
        varying float vSize;
        void main() {
          vec2 p = gl_PointCoord - vec2(0.5);
          float d = dot(p,p);
          float a = smoothstep(0.25, 0.0, d) * uOpacity;
          gl_FragColor = vec4(vec3(1.0), a);
        }
      `,
    });
    const stars = new THREE.Points(starsGeo, starsMat);
    scene.add(stars);

    // Points on/around planet
    const dotGeo = new THREE.SphereGeometry(0.045, 14, 12);
    const dotMat = new THREE.MeshStandardMaterial({
      color: new THREE.Color("#88e0ff"),
      emissive: new THREE.Color("#88e0ff"),
      emissiveIntensity: 0.9,
      roughness: 0.25,
      metalness: 0.15,
      transparent: true,
      opacity: 0.9,
    });

    const totalDots = prepared.fill.length + prepared.clickable.length;
    const dots = new THREE.InstancedMesh(dotGeo, dotMat, totalDots);
    dots.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
    root.add(dots);

    const colors = new Float32Array(totalDots * 3);
    const tmpMat4 = new THREE.Matrix4();
    const tmpQuat = new THREE.Quaternion();
    const tmpScale = new THREE.Vector3(1, 1, 1);
    const tmpPos = new THREE.Vector3();

    const instanceToNode = new Map<number, SquarePlanetNode>();
    const basePositions: Array<{ v: THREE.Vector3; hue: number; label?: string; node?: SquarePlanetNode }> = [];

    // Fillers first
    for (let i = 0; i < prepared.fill.length; i++) {
      const p = prepared.fill[i].pos.clone().multiplyScalar(planetRadius * 1.12);
      basePositions.push({ v: p, hue: prepared.fill[i].hue });
    }
    for (let i = 0; i < prepared.clickable.length; i++) {
      const c = prepared.clickable[i];
      const p = c.pos.clone().multiplyScalar(planetRadius * 1.22);
      basePositions.push({ v: p, hue: c.hue, label: c.node.label, node: c.node });
      instanceToNode.set(prepared.fill.length + i, c.node);
    }

    for (let i = 0; i < basePositions.length; i++) {
      const bp = basePositions[i];
      const s = bp.node ? 1.2 : 1.0;
      tmpScale.setScalar(bp.node ? s : lerp(0.85, 1.05, (i % 19) / 18));
      tmpPos.copy(bp.v);
      tmpMat4.compose(tmpPos, tmpQuat, tmpScale);
      dots.setMatrixAt(i, tmpMat4);

      const c = new THREE.Color().setHSL((bp.hue % 360) / 360, 0.9, bp.node ? 0.72 : 0.62);
      colors[i * 3 + 0] = c.r;
      colors[i * 3 + 1] = c.g;
      colors[i * 3 + 2] = c.b;
    }
    dots.instanceColor = new THREE.InstancedBufferAttribute(colors, 3);
    dots.instanceColor.needsUpdate = true;

    const raycaster = new THREE.Raycaster();
    const pointerNdc = new THREE.Vector2();
    let clickCandidate: { x: number; y: number } | null = null;
    let dragging = false;

    const state = {
      rotX: -0.25,
      rotY: 0.55,
      velX: 0,
      velY: 0.004,
      ring: 0,
      hidden: false,
      inView: true,
    };

    function onPointerDown(e: PointerEvent) {
      if (disposed) return;
      dragging = true;
      clickCandidate = { x: e.clientX, y: e.clientY };
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    }

    function onPointerMove(e: PointerEvent) {
      if (!dragging) return;
      if (!clickCandidate) clickCandidate = { x: e.clientX, y: e.clientY };
      const dx = e.movementX || 0;
      const dy = e.movementY || 0;
      state.rotY += dx * 0.008;
      state.rotX += dy * 0.006;
      state.velY = dx * 0.0012;
      state.velX = dy * 0.0009;
      if (clickCandidate) {
        const moved = Math.abs(e.clientX - clickCandidate.x) + Math.abs(e.clientY - clickCandidate.y);
        if (moved > 10) clickCandidate = null;
      }
    }

    function onPointerUp(e: PointerEvent) {
      dragging = false;
      if (disposed) return;
      if (!clickCandidate) return;
      const rect = renderer.domElement.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;
      pointerNdc.x = (x / rect.width) * 2 - 1;
      pointerNdc.y = -(y / rect.height) * 2 + 1;
      raycaster.setFromCamera(pointerNdc, camera);
      const hits = raycaster.intersectObject(dots, true);
      const hit = hits.find((h: THREE.Intersection) => typeof (h as any).instanceId === "number");
      const instanceId = (hit as any)?.instanceId as number | undefined;
      if (instanceId === undefined) return;
      const node = instanceToNode.get(instanceId);
      if (node) onSelect(node);
    }

    renderer.domElement.style.touchAction = "none";
    renderer.domElement.addEventListener("pointerdown", onPointerDown);
    renderer.domElement.addEventListener("pointermove", onPointerMove);
    renderer.domElement.addEventListener("pointerup", onPointerUp);
    function onPointerCancel() {
      dragging = false;
      clickCandidate = null;
    }
    renderer.domElement.addEventListener("pointercancel", onPointerCancel);

    const ro = new ResizeObserver(() => {
      if (disposed) return;
      const rect = mountEl.getBoundingClientRect();
      const w = Math.max(1, Math.floor(rect.width));
      const h = Math.max(1, Math.floor(rect.height));
      const dpr = clamp(window.devicePixelRatio || 1, 1, 2);
      renderer.setPixelRatio(dpr);
      renderer.setSize(w, h, false);
      overlay.width = Math.floor(w * dpr);
      overlay.height = Math.floor(h * dpr);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    });
    ro.observe(mountEl);

    const io = new IntersectionObserver(
      (entries) => {
        const v = entries[0]?.isIntersecting ?? true;
        state.inView = v;
      },
      { threshold: 0.05 },
    );
    io.observe(mountEl);

    const onVis = () => {
      state.hidden = document.visibilityState !== "visible";
    };
    document.addEventListener("visibilitychange", onVis);

    let raf = 0;
    let lastTs = 0;

    function renderLabels(width: number, height: number) {
      if (!overlayCtx) return;
      const dpr = clamp(window.devicePixelRatio || 1, 1, 2);
      overlayCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
      overlayCtx.clearRect(0, 0, width, height);

      overlayCtx.textBaseline = "middle";
      overlayCtx.textAlign = "left";

      let shown = 0;
      for (let i = prepared.fill.length; i < basePositions.length; i++) {
        const bp = basePositions[i];
        if (!bp.node || !bp.label) continue;

        tmpPos.copy(bp.v).applyEuler(root.rotation as any);
        tmpPos.project(camera);
        if (tmpPos.z < -1 || tmpPos.z > 1) continue;
        if (tmpPos.z > 0.35) continue; // too far back

        const sx = ((tmpPos.x + 1) / 2) * width;
        const sy = ((-tmpPos.y + 1) / 2) * height;

        const depth = clamp(1 - (tmpPos.z + 1) / 2, 0, 1);
        const fontSize = Math.round(lerp(10, 13, depth));
        overlayCtx.font = `${fontSize}px ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial`;

        const text = bp.label.length > 10 ? `${bp.label.slice(0, 10)}…` : bp.label;
        const tw = overlayCtx.measureText(text).width;
        const padX = 8;
        const padY = 5;
        const bx = sx + 12;
        const by = sy;

        overlayCtx.globalAlpha = lerp(0.25, 0.85, depth);
        overlayCtx.fillStyle = "rgba(0,0,0,0.35)";
        overlayCtx.beginPath();
        roundRectPath(overlayCtx, bx - padX, by - fontSize / 2 - padY, tw + padX * 2, fontSize + padY * 2, 10);
        overlayCtx.fill();

        overlayCtx.globalAlpha = lerp(0.35, 0.92, depth);
        overlayCtx.fillStyle = "rgba(240,245,255,0.95)";
        overlayCtx.fillText(text, bx, by);

        shown++;
        if (shown > 18) break;
      }
      overlayCtx.globalAlpha = 1;
    }

    function tick(ts: number) {
      raf = requestAnimationFrame(tick);
      if (disposed) return;
      if (state.hidden || !state.inView) return;

      const dt = lastTs ? clamp((ts - lastTs) / 16.67, 0.2, 3.0) : 1;
      lastTs = ts;

      // auto rotation + inertia
      if (!prefersReducedMotion && !dragging) state.velY += 0.00018 * dt;
      state.velX *= Math.pow(0.92, dt);
      state.velY *= Math.pow(0.94, dt);
      state.rotX += state.velX * dt;
      state.rotY += state.velY * dt;
      state.ring += 0.012 * dt;

      root.rotation.x = state.rotX;
      root.rotation.y = state.rotY;
      ring.rotation.z = state.ring;
      stars.rotation.y += 0.0007 * dt;

      renderer.render(scene, camera);

      const rect = mountEl.getBoundingClientRect();
      renderLabels(Math.max(1, rect.width), Math.max(1, rect.height));
    }

    raf = requestAnimationFrame(tick);

    return () => {
      disposed = true;
      cancelAnimationFrame(raf);
      document.removeEventListener("visibilitychange", onVis);
      io.disconnect();
      ro.disconnect();
      renderer.domElement.removeEventListener("pointerdown", onPointerDown);
      renderer.domElement.removeEventListener("pointermove", onPointerMove);
      renderer.domElement.removeEventListener("pointerup", onPointerUp);
      renderer.domElement.removeEventListener("pointercancel", onPointerCancel);

      scene.traverse((obj: THREE.Object3D) => {
        const mesh = obj as THREE.Mesh;
        if ((mesh as any).geometry) (mesh as any).geometry.dispose?.();
        const material = (mesh as any).material;
        if (Array.isArray(material)) material.forEach((m) => m.dispose?.());
        else material?.dispose?.();
      });
      renderer.dispose();

      try {
        mountEl.removeChild(container);
      } catch (e) {
        console.debug("[AIHub] SquarePlanetThree cleanup failed", e);
      }
    };
  }, [prepared, onSelect]);

  return <div ref={rootRef} className={className ?? ""} />;
}
