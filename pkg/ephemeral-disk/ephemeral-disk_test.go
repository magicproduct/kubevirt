/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2017 Red Hat, Inc.
 *
 */

package ephemeraldisk

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/kubevirt/pkg/libvmi"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
)

var _ = Describe("ContainerDisk", func() {
	var (
		cowImageBaseDir   string // Where COW images are created (creator.mountBaseDir)
		pvcSourceBaseDir  string // Where source PVC filesystem images are mocked (creator.pvcBaseDir)
		blockDevSourceDir string // Where source block devices are mocked (creator.blockDevBaseDir)
		creator           *ephemeralDiskCreator
	)

	// createBackingImageForPVC is the original helper for existing tests.
	// It creates the source backing file at the default path ("disk.img").
	createBackingImageForPVC := func(volumeName string, isBlock bool) error { // Keep this for old tests
		// For filesystem, ensure /pvcSourceBaseDir/volumeName/ exists
		if !isBlock {
			if err := os.MkdirAll(filepath.Join(pvcSourceBaseDir, volumeName), 0755); err != nil && !os.IsExist(err) {
				return err
			}
		}
		// Create the source backing file using the default path logic in getBackingFilePath
		// For filesystem, this will be /pvcSourceBaseDir/volumeName/disk.img
		// For block, this will be /blockDevSourceDir/volumeName
		f, err := os.Create(creator.getBackingFilePath(volumeName, "", isBlock))
		if err != nil {
			return err
		}
		defer f.Close()

		// Test the test infra itself: make sure that the backing file has been created.
		if isBlock {
			if _, err := os.Stat(filepath.Join(blockDevSourceDir, volumeName)); err != nil {
				return err
			}
		} else {
			if _, err := os.Stat(filepath.Join(pvcSourceBaseDir, volumeName, "disk.img")); err != nil {
				return err
			}
		}
		return nil
	}

	// fakeCreateBackingDiskWithFormatTracking simulates qemu-img create, tracks the backingFormat,
	// and ensures the source backingFile exists and the target imagePath (COW) is created.
	fakeCreateBackingDiskWithFormatTracking := func(backingFile string, backingFormat ephemeralDiskFormat, imagePath string) ([]byte, error) {
		if _, err := os.Stat(backingFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("source backing file %s does not exist: %w", backingFile, err)
		} else if err != nil {
			return nil, fmt.Errorf("error stating source backing file %s: %w", backingFile, err)
		}

		if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil && !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create parent dir for COW image %s: %w", imagePath, err)
		}
		f, err := os.Create(imagePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create COW image %s: %w", imagePath, err)
		}
		err = f.Close()
		return nil, err
	}

	BeforeEach(func() {
		cowImageBaseDir = GinkgoT().TempDir()
		pvcSourceBaseDir = GinkgoT().TempDir()
		blockDevSourceDir = GinkgoT().TempDir()

		creator = &ephemeralDiskCreator{
			mountBaseDir:    cowImageBaseDir,
			pvcBaseDir:      pvcSourceBaseDir,
			blockDevBaseDir: blockDevSourceDir,
			discCreateFunc:  fakeCreateBackingDiskWithFormatTracking,
		}
		// creator.Init() would normally create this directory.
		Expect(os.MkdirAll(creator.mountBaseDir, 0755)).To(Succeed())
	})

	Describe("ephemeral-backed PVC", func() {
		Context("With single ephemeral volume", func() {
			It("Should create VirtualMachineInstance's ephemeral image", func() {
				By("Creating a minimal VirtualMachineInstance object with single ephemeral-backed PVC")
				vmi := libvmi.New(
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk", "fake-pvc", "", ""),
				)

				By("Creating a backing image for the PVC")
				Expect(createBackingImageForPVC("fake-disk", false)).To(Succeed())

				By("Creating VirtualMachineInstance disk image that corresponds to the VMIs PVC")
				err := creator.CreateEphemeralImages(vmi, &api.Domain{})
				Expect(err).NotTo(HaveOccurred())

				// Now we can test the behavior - the COW image must exist.
				_, err = os.Stat(filepath.Join(creator.mountBaseDir, "fake-disk", "disk.qcow2"))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("With multiple ephemeral volumes", func() {
			It("Should create VirtualMachineInstance's ephemeral images", func() {
				By("Creating a minimal VirtualMachineInstance object with multiple ephemeral-backed PVC")
				vmi := libvmi.New(
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk1", "fake-pvc1", "", ""),
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk2", "fake-pvc2", "", ""),
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk3", "fake-pvc3", "", ""),
				)

				By("Creating a backing images for the PVC")
				Expect(createBackingImageForPVC("fake-disk1", false)).To(Succeed())
				Expect(createBackingImageForPVC("fake-disk2", false)).To(Succeed())
				Expect(createBackingImageForPVC("fake-disk3", false)).To(Succeed())

				By("Creating VirtualMachineInstance disk image that corresponds to the VMIs PVC")
				err := creator.CreateEphemeralImages(vmi, &api.Domain{})
				Expect(err).NotTo(HaveOccurred())

				// Now we can test the behavior - the COW image must exist.
				_, err = os.Stat(filepath.Join(creator.mountBaseDir, "fake-disk1", "disk.qcow2"))
				Expect(err).NotTo(HaveOccurred())
				_, err = os.Stat(filepath.Join(creator.mountBaseDir, "fake-disk2", "disk.qcow2"))
				Expect(err).NotTo(HaveOccurred())
				_, err = os.Stat(filepath.Join(creator.mountBaseDir, "fake-disk3", "disk.qcow2"))
				Expect(err).NotTo(HaveOccurred())
			})
			It("Should create ephemeral images in an idempotent way", func() {
				By("Creating a minimal VirtualMachineInstance object with single ephemeral-backed PVC")
				vmi := libvmi.New(
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk", "fake-pvc", "", ""),
				)

				By("Creating a backing image for the PVC")
				Expect(createBackingImageForPVC("fake-disk", false)).To(Succeed())

				err := creator.CreateEphemeralImages(vmi, &api.Domain{})
				Expect(err).NotTo(HaveOccurred())
				err = creator.CreateEphemeralImages(vmi, &api.Domain{})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("With a block pvc backed ephemeral volume", func() {
			It("Should create VirtualMachineInstance's ephemeral image", func() {
				By("Creating a minimal VirtualMachineInstance object with single ephemeral-backed PVC")
				vmi := libvmi.New(
					libvmi.WithEphemeralPersistentVolumeClaim("fake-disk", "fake-pvc", "", ""),
				)

				By("Creating a backing images for the PVC")
				Expect(createBackingImageForPVC("fake-disk", true)).To(Succeed())

				By("Creating VirtualMachineInstance disk image that corresponds to the VMIs PVC")
				err := creator.CreateEphemeralImages(vmi, &api.Domain{
					Spec: api.DomainSpec{
						Devices: api.Devices{
							Disks: []api.Disk{
								{
									BackingStore: &api.BackingStore{
										Type: "block",
										Source: &api.DiskSource{
											Dev:  filepath.Join(creator.blockDevBaseDir, "fake-disk"),
											Name: "fake-disk",
										},
									},
								},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())

				// Now we can test the behavior - the COW image must exist.
				_, err = os.Stat(filepath.Join(creator.mountBaseDir, "fake-disk", "disk.qcow2"))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

func fakeCreateBackingSource(volumeName string, imagePathInPVC string, isBlock bool, blockDevPath string, pvcBasePath string) (string, error) {
	var actualBackingFilePath string

	if isBlock {
		actualBackingFilePath = filepath.Join(blockDevPath, volumeName)
		if err := os.MkdirAll(filepath.Dir(actualBackingFilePath), 0755); err != nil && !os.IsExist(err) {
			return "", err
		}
	} else {
		pvcVolumeMountDir := filepath.Join(pvcBasePath, volumeName)
		if err := os.MkdirAll(pvcVolumeMountDir, 0755); err != nil && !os.IsExist(err) {
			return "", err
		}
		pathInPvcToUse := imagePathInPVC
		if pathInPvcToUse == "" {
			pathInPvcToUse = "disk.img" // Default path
		}
		actualBackingFilePath = filepath.Join(pvcVolumeMountDir, pathInPvcToUse)
		if dir := filepath.Dir(actualBackingFilePath); dir != pvcVolumeMountDir {
			if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
				return "", err
			}
		}
	}

	f, createErr := os.Create(actualBackingFilePath)
	if createErr != nil {
		return "", fmt.Errorf("failed to create fake backing file %s: %w", actualBackingFilePath, createErr)
	}
	f.Close()
	if _, statErr := os.Stat(actualBackingFilePath); statErr != nil {
		return "", fmt.Errorf("fake backing file %s was not created: %w", actualBackingFilePath, statErr)
	}
	return actualBackingFilePath, nil
}

var _ = Describe("EphemeralDisk with Type and ImagePath", func() {
	var (
		cowImageBaseDir   string // Where COW images are created (creator.mountBaseDir)
		pvcSourceBaseDir  string // Where source PVC filesystem images are mocked (creator.pvcBaseDir)
		blockDevSourceDir string // Where source block devices are mocked (creator.blockDevBaseDir)
		creator           *ephemeralDiskCreator
		lastBackingFormat ephemeralDiskFormat // Stores the format passed to the fake disk creation func
	)

	fakeCreateBackingDiskWithFormatTracking := func(backingFile string, backingFormat ephemeralDiskFormat, imagePath string) ([]byte, error) {
		lastBackingFormat = backingFormat
		if _, err := os.Stat(backingFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("source backing file %s does not exist: %w", backingFile, err)
		} else if err != nil {
			return nil, fmt.Errorf("error stating source backing file %s: %w", backingFile, err)
		}
		if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil && !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create parent dir for COW image %s: %w", imagePath, err)
		}
		f, err := os.Create(imagePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create COW image %s: %w", imagePath, err)
		}
		return nil, f.Close()
	}

	BeforeEach(func() {
		cowImageBaseDir = GinkgoT().TempDir()
		pvcSourceBaseDir = GinkgoT().TempDir()
		blockDevSourceDir = GinkgoT().TempDir()
		lastBackingFormat = ""

		creator = &ephemeralDiskCreator{
			mountBaseDir:    cowImageBaseDir,
			pvcBaseDir:      pvcSourceBaseDir,
			blockDevBaseDir: blockDevSourceDir,
			discCreateFunc:  fakeCreateBackingDiskWithFormatTracking,
		}
		Expect(os.MkdirAll(creator.mountBaseDir, 0755)).To(Succeed())
	})

	Context("For filesystem-backed PVCs", func() {
		It("should use raw format and default path if not specified", func() {
			volumeName := "testvol-fs-default"
			vmi := libvmi.New(libvmi.WithEphemeralPersistentVolumeClaim(volumeName, "fake-pvc", "", ""))
			expectedBackingSourcePath := filepath.Join(pvcSourceBaseDir, volumeName, "disk.img")
			_, err := fakeCreateBackingSource(volumeName, "", false, blockDevSourceDir, pvcSourceBaseDir)
			Expect(err).NotTo(HaveOccurred())

			err = creator.CreateEphemeralImages(vmi, &api.Domain{})
			Expect(err).NotTo(HaveOccurred())
			Expect(creator.GetFilePath(volumeName)).To(BeARegularFile())
			Expect(lastBackingFormat).To(Equal(ephemeralDiskFormatRAW))
			Expect(creator.getBackingFilePath(volumeName, "", false)).To(Equal(expectedBackingSourcePath))
		})

		It("should use specified qcow2 type and custom path", func() {
			volumeName := "testvol-fs-qcow2-custom"
			customPath := "custom/path/myimage.qcow2"
			vmi := libvmi.New(libvmi.WithEphemeralPersistentVolumeClaim(volumeName, "fake-pvc", "qcow2", customPath))
			expectedBackingSourcePath := filepath.Join(pvcSourceBaseDir, volumeName, customPath)
			_, err := fakeCreateBackingSource(volumeName, customPath, false, blockDevSourceDir, pvcSourceBaseDir)
			Expect(err).NotTo(HaveOccurred())

			err = creator.CreateEphemeralImages(vmi, &api.Domain{})
			Expect(err).NotTo(HaveOccurred())
			Expect(creator.GetFilePath(volumeName)).To(BeARegularFile())
			Expect(lastBackingFormat).To(Equal(ephemeralDiskFormatQCow2))
			Expect(creator.getBackingFilePath(volumeName, customPath, false)).To(Equal(expectedBackingSourcePath))
		})
	})

	Context("With invalid configuration", func() {
		It("should fail for an unknown image type", func() {
			volumeName := "testvol-invalid-type"
			vmi := libvmi.New(libvmi.WithEphemeralPersistentVolumeClaim(volumeName, "fake-pvc", "unknownformat", ""))
			_, err := fakeCreateBackingSource(volumeName, "", false, blockDevSourceDir, pvcSourceBaseDir)
			Expect(err).NotTo(HaveOccurred())

			err = creator.CreateEphemeralImages(vmi, &api.Domain{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown image type 'unknownformat'"))
		})

		It("should fail if backing file does not exist for filesystem PVC", func() {
			volumeName := "testvol-no-backing-fs"
			vmi := libvmi.New(libvmi.WithEphemeralPersistentVolumeClaim(volumeName, "fake-pvc", "raw", "some/path.img"))
			// DO NOT create backing file using createActualBackingFile
			err := creator.CreateEphemeralImages(vmi, &api.Domain{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(MatchRegexp(`source backing file .*` + pvcSourceBaseDir + `/.*/some/path.img does not exist`))
		})
	})
})
