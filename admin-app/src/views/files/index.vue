<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <!-- 头部栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-zinc-200 dark:border-zinc-800">
      <div>
        <h2 class="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
          <component
            :is="FolderIcon"
            class="w-5 h-5 text-zinc-600 dark:text-zinc-400"
          />
          文件管理
        </h2>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          浏览本地存储目录、上传资产包、下载文件与维护磁盘文件树
        </p>
      </div>

      <div class="flex items-center gap-2">
        <el-upload
          :show-file-list="false"
          :before-upload="handleUpload"
        >
          <el-button
            type="primary"
            size="default"
            :loading="uploading"
          >
            <template #icon>
              <component
                :is="UploadIcon"
                class="w-4 h-4"
              />
            </template>
            上传文件
          </el-button>
        </el-upload>
      </div>
    </div>

    <!-- 面包屑路径条与返回上一级 -->
    <div class="p-3.5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 flex items-center justify-between gap-4 shadow-sm">
      <div class="flex items-center gap-2 overflow-x-auto text-xs">
        <button
          class="flex items-center gap-1 text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors shrink-0"
          @click="navigateTo('')"
        >
          <component
            :is="HomeIcon"
            class="w-3.5 h-3.5"
          />
          <span class="font-medium">根目录</span>
        </button>

        <template
          v-for="(part, idx) in pathParts"
          :key="idx"
        >
          <span class="text-zinc-300 dark:text-zinc-700">/</span>
          <button
            class="text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors shrink-0 font-medium truncate max-w-[160px]"
            @click="navigateTo(part.path)"
          >
            {{ part.name }}
          </button>
        </template>
      </div>

      <div
        v-if="currentPath"
        class="shrink-0"
      >
        <el-button
          size="small"
          @click="navigateParent"
        >
          <template #icon>
            <component
              :is="CornerLeftUpIcon"
              class="w-3.5 h-3.5"
            />
          </template>
          返回上一级
        </el-button>
      </div>
    </div>

    <!-- 文件表格 (桌面端) -->
    <div class="hidden sm:block rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden shadow-sm">
      <el-table
        v-loading="loading"
        :data="files"
        style="width: 100%"
        empty-text="此目录下暂无文件"
      >
        <el-table-column
          label="名称"
          min-width="260"
        >
          <template #default="{ row }">
            <div class="flex items-center gap-2.5">
              <component
                :is="row.is_dir ? FolderIcon : FileTextIcon"
                class="w-4 h-4 shrink-0"
                :class="row.is_dir ? 'text-amber-500' : 'text-zinc-400'"
              />
              <a
                v-if="row.is_dir"
                class="font-medium text-xs text-zinc-800 dark:text-zinc-200 hover:text-zinc-950 dark:hover:text-zinc-50 cursor-pointer"
                @click="navigateTo(getItemPath(row.name))"
              >
                {{ row.name }}
              </a>
              <span
                v-else
                class="text-xs text-zinc-700 dark:text-zinc-300 select-text"
              >
                {{ row.name }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="类型"
          width="100"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-500">
              {{ row.is_dir ? '文件夹' : '文件' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="大小"
          width="120"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-500 font-mono">
              {{ row.is_dir ? '-' : formatSize(row.size) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="修改时间"
          width="180"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-500">
              {{ formatDate(row.mod_time) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="操作"
          width="160"
          fixed="right"
        >
          <template #default="{ row }">
            <div class="flex items-center gap-1">
              <el-button
                v-if="!row.is_dir"
                size="small"
                text
                @click="handleDownload(getItemPath(row.name))"
              >
                <template #icon>
                  <component
                    :is="DownloadIcon"
                    class="w-3.5 h-3.5"
                  />
                </template>
                下载
              </el-button>

              <el-popconfirm
                title="确定删除此项吗？该操作不可恢复！"
                confirm-button-text="确定"
                cancel-button-text="取消"
                @confirm="handleDelete(getItemPath(row.name))"
              >
                <template #reference>
                  <el-button
                    size="small"
                    type="danger"
                    text
                  >
                    <template #icon>
                      <component
                        :is="Trash2Icon"
                        class="w-3.5 h-3.5"
                      />
                    </template>
                    删除
                  </el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 移动端卡片式展示 -->
    <div class="sm:hidden space-y-3">
      <div
        v-if="files.length === 0 && !loading"
        class="py-12 text-center text-xs text-zinc-400"
      >
        暂无文件
      </div>

      <div
        v-for="file in files"
        :key="file.name"
        class="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 space-y-2 shadow-sm"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0">
            <component
              :is="file.is_dir ? FolderIcon : FileTextIcon"
              class="w-4 h-4 shrink-0"
              :class="file.is_dir ? 'text-amber-500' : 'text-zinc-400'"
            />
            <a
              v-if="file.is_dir"
              class="font-medium text-xs text-zinc-900 dark:text-zinc-100 truncate cursor-pointer"
              @click="navigateTo(getItemPath(file.name))"
            >
              {{ file.name }}
            </a>
            <span
              v-else
              class="text-xs text-zinc-900 dark:text-zinc-100 truncate font-medium"
            >
              {{ file.name }}
            </span>
          </div>

          <div class="flex items-center gap-1 shrink-0">
            <el-button
              v-if="!file.is_dir"
              size="small"
              text
              @click="handleDownload(getItemPath(file.name))"
            >
              <component
                :is="DownloadIcon"
                class="w-3.5 h-3.5"
              />
            </el-button>
            <el-popconfirm
              title="确定删除此项吗？"
              confirm-button-text="确定"
              cancel-button-text="取消"
              @confirm="handleDelete(getItemPath(file.name))"
            >
              <template #reference>
                <el-button
                  size="small"
                  type="danger"
                  text
                >
                  <component
                    :is="Trash2Icon"
                    class="w-3.5 h-3.5"
                  />
                </el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>

        <div class="flex items-center justify-between text-[11px] text-zinc-400 pt-1">
          <span>{{ file.is_dir ? '目录' : formatSize(file.size) }}</span>
          <span>{{ formatDate(file.mod_time) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getFiles, deleteFile, downloadFile, uploadFile } from '@/api/files'
import type { FileInfo } from '@/types'
import { formatSize } from '@/lib/utils'
import dayjs from 'dayjs'
import {
  Folder,
  FileText,
  Upload,
  Download,
  Trash2,
  Home,
  CornerLeftUp
} from 'lucide-vue-next'

const FolderIcon = Folder
const FileTextIcon = FileText
const UploadIcon = Upload
const DownloadIcon = Download
const Trash2Icon = Trash2
const HomeIcon = Home
const CornerLeftUpIcon = CornerLeftUp

const loading = ref(false)
const uploading = ref(false)
const files = ref<FileInfo[]>([])
const currentPath = ref('')

const pathParts = computed(() => {
  if (!currentPath.value) return []
  const parts = currentPath.value.split('/')
  return parts.map((name, index) => ({
    name,
    path: parts.slice(0, index + 1).join('/')
  }))
})

const getItemPath = (name: string) => {
  return currentPath.value ? `${currentPath.value}/${name}` : name
}

const formatDate = (dateStr: string) => {
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm')
}

const loadFiles = async (path: string) => {
  loading.value = true
  try {
    const list = await getFiles(path)
    files.value = list
    currentPath.value = path
  } catch (error: any) {
    ElMessage.error(error.message || '加载文件列表失败')
  } finally {
    loading.value = false
  }
}

const navigateTo = (path: string) => {
  loadFiles(path)
}

const navigateParent = () => {
  if (!currentPath.value) return
  const parent = currentPath.value.split('/').slice(0, -1).join('/')
  loadFiles(parent)
}

const handleUpload = async (file: File) => {
  uploading.value = true
  const targetPath = getItemPath(file.name)
  try {
    await uploadFile(targetPath, file)
    ElMessage.success(`上传 ${file.name} 成功`)
    await loadFiles(currentPath.value)
  } catch (error: any) {
    ElMessage.error(error.message || `上传 ${file.name} 失败`)
  } finally {
    uploading.value = false
  }
  return false
}

const handleDownload = async (path: string) => {
  try {
    const blob = await downloadFile(path)
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = path.split('/').pop() || 'file'
    document.body.appendChild(a)
    a.click()
    setTimeout(() => {
      window.URL.revokeObjectURL(url)
      a.remove()
    }, 4000)
  } catch (error: any) {
    ElMessage.error(error.message || '下载失败')
  }
}

const handleDelete = async (path: string) => {
  try {
    await deleteFile(path)
    ElMessage.success('删除成功')
    await loadFiles(currentPath.value)
  } catch (error: any) {
    ElMessage.error(error.message || '删除失败')
  }
}

onMounted(() => {
  loadFiles('')
})
</script>
